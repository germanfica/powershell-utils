#!/usr/bin/env node
'use strict';

const fs = require('fs');
const fsp = fs.promises;
const path = require('path');

const printHelp = () => {
  console.log(`
Uso:
  node organize-by-ext.js <directorio> [opciones]

Opciones:
  --rules="jpg,png=Imagenes;pdf,docx=Documentos;mp3=Audio"
      Reglas de agrupado. Formato: "ext1,ext2=Carpeta;ext3=OtraCarpeta"
      (sin puntos, separadas por coma, grupos separados por ';')

  --recursive
      Escanea subcarpetas tambien (los archivos se mueven igual a carpetas dentro del directorio base).

  --dry-run
      No mueve nada, solo muestra lo que haria.

  --ignore= "node_modules,.git,Dist"
      Carpetas a ignorar (nombres, no rutas). Default: "node_modules,.git"

Ejemplos:
  node organize-by-ext.js ./Descargas
  node organize-by-ext.js ./Descargas --rules="jpg,png=Imagenes;pdf=PDFs" --dry-run
  node organize-by-ext.js "C:\\Users\\William\\Downloads" --recursive
`.trim());
};

const parseKVArgs = (argv) => {
  const out = { _: [] };
  argv.forEach((a) => {
    if (!a.startsWith('--')) out._.push(a);
    else {
      const [k, v] = a.split('=');
      out[k] = v === undefined ? true : v;
    }
  });
  return out;
};

const normalizeExt = (ext) => ext.trim().toLowerCase().replace(/^\./, '');

const parseRules = (rulesStr) => {
  // "jpg,png=Imagenes;pdf=PDFs"
  const rules = new Map(); // ext -> folderName
  if (!rulesStr) return rules;

  const groups = rulesStr.split(';').map((s) => s.trim()).filter(Boolean);
  groups.forEach((g) => {
    const eq = g.indexOf('=');
    if (eq === -1) return;
    const left = g.slice(0, eq).trim();
    const folder = g.slice(eq + 1).trim();
    if (!left || !folder) return;

    left.split(',').map(normalizeExt).filter(Boolean).forEach((ext) => {
      rules.set(ext, folder);
    });
  });

  return rules;
};

const ensureDir = async (dirPath) => {
  await fsp.mkdir(dirPath, { recursive: true });
};

const moveFile = async (src, dst) => {
  try {
    await fsp.rename(src, dst);
  } catch (err) {
    // Fallback si es otro filesystem (EXDEV)
    if (err && err.code === 'EXDEV') {
      await fsp.copyFile(src, dst);
      await fsp.unlink(src);
      return;
    }
    throw err;
  }
};

const uniquePathIfExists = async (dstPath) => {
  // Si existe, agrega " (1)", " (2)", etc.
  const exists = async (p) => {
    try { await fsp.access(p); return true; } catch { return false; }
  };

  if (!(await exists(dstPath))) return dstPath;

  const dir = path.dirname(dstPath);
  const ext = path.extname(dstPath);
  const base = path.basename(dstPath, ext);

  let i = 1;
  while (true) {
    const candidate = path.join(dir, `${base} (${i})${ext}`);
    if (!(await exists(candidate))) return candidate;
    i += 1;
  }
};

const walk = async (dir, opts, onFile) => {
  const entries = await fsp.readdir(dir, { withFileTypes: true });

  for (const ent of entries) {
    const full = path.join(dir, ent.name);

    if (ent.isDirectory()) {
      if (opts.ignore.has(ent.name)) continue;
      if (!opts.recursive) continue;
      await walk(full, opts, onFile);
      continue;
    }

    if (ent.isFile()) {
      await onFile(full);
    }
  }
};

const run = async () => {
  const args = parseKVArgs(process.argv.slice(2));

  if (args._.length === 0 || args['--help'] || args['-h']) {
    printHelp();
    process.exit(args._.length === 0 ? 1 : 0);
  }

  const baseDir = path.resolve(args._[0]);
  const dryRun = Boolean(args['--dry-run']);
  const recursive = Boolean(args['--recursive']);

  const ignoreStr = (args['--ignore'] || 'node_modules,.git').toString();
  const ignore = new Set(ignoreStr.split(',').map((s) => s.trim()).filter(Boolean));

  const rules = parseRules(args['--rules'] ? String(args['--rules']) : '');
  const usingRules = rules.size > 0;

  // Destinos creados (para ignorarlos si estas en recursive)
  const createdDestFolders = new Set();

  const decideFolder = (ext) => {
    if (!ext) return null;
    if (usingRules) return rules.get(ext) || null;
    return ext; // por default: carpeta con el nombre de la extension
  };

  const safeIgnoreHas = (name) => ignore.has(name) || createdDestFolders.has(name);

  const opts = { dryRun, recursive, ignore: new Set() };

  // usamos un "proxy" para ignore porque createdDestFolders cambia en runtime
  Object.defineProperty(opts, 'ignore', {
    get: () => ({
      has: (name) => safeIgnoreHas(name),
    }),
  });

  let moved = 0;
  let skipped = 0;

  // Validar baseDir
  const st = await fsp.stat(baseDir).catch(() => null);
  if (!st || !st.isDirectory()) {
    console.error(`El directorio no existe o no es un directorio: ${baseDir}`);
    process.exit(2);
  }

  const onFile = async (filePath) => {
    const rel = path.relative(baseDir, filePath);

    // Evitar tocar cosas fuera (por las dudas)
    if (rel.startsWith('..') || path.isAbsolute(rel)) return;

    const ext = normalizeExt(path.extname(filePath));
    const folderName = decideFolder(ext);

    if (!folderName) {
      skipped += 1;
      return;
    }

    const destDir = path.join(baseDir, folderName);
    const destPath0 = path.join(destDir, path.basename(filePath));

    // Si ya esta dentro de la carpeta destino, saltear
    if (path.dirname(filePath) === destDir) {
      skipped += 1;
      return;
    }

    if (recursive) createdDestFolders.add(folderName);

    if (!dryRun) await ensureDir(destDir);
    const destPath = dryRun ? destPath0 : await uniquePathIfExists(destPath0);

    const action = dryRun ? '[DRY]' : '[MOVE]';
    console.log(`${action} ${filePath} -> ${destPath}`);

    if (!dryRun) {
      await moveFile(filePath, destPath);
    }
    moved += 1;
  };

  await walk(baseDir, { recursive, ignore: { has: (n) => safeIgnoreHas(n) } }, onFile);

  console.log(`\nListo. Movidos: ${moved} | Saltados: ${skipped} | Base: ${baseDir}`);
};

run().catch((err) => {
  console.error('Error:', err && err.message ? err.message : err);
  process.exit(1);
});
