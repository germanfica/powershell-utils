#!/usr/bin/env node
'use strict';

const fs = require('fs');
const fsp = require('fs/promises');
const path = require('path');
const readline = require('readline');

const FENCE = '```';
const DEFAULT_OUT = 'output.txt';
const DEFAULT_EXCLUDES = new Set(['node_modules', 'dist']);

const isExcludedPath = (p, excludes) => {
  const normalized = path.normalize(p);
  const parts = normalized.split(path.sep).filter(Boolean);
  return parts.some((part) => excludes.has(part));
};

const isTsFile = (p) => p.toLowerCase().endsWith('.ts');
const isSpecTsFile = (p) => p.toLowerCase().endsWith('.spec.ts');

const statSafe = async (p) => {
  try {
    return await fsp.stat(p);
  } catch {
    return null;
  }
};

const walkForTsFiles = async (root, excludes) => {
  const results = [];

  const st = await statSafe(root);
  if (!st) return results;

  if (st.isFile()) {
    if (isTsFile(root) && !isSpecTsFile(root) && !isExcludedPath(root, excludes)) {
      results.push(root);
    }
    return results;
  }

  if (!st.isDirectory()) return results;
  if (isExcludedPath(root, excludes)) return results;

  const entries = await fsp.readdir(root, { withFileTypes: true });

  for (const ent of entries) {
    const full = path.join(root, ent.name);

    if (ent.isDirectory()) {
      if (!excludes.has(ent.name) && !isExcludedPath(full, excludes)) {
        const nested = await walkForTsFiles(full, excludes);
        results.push(...nested);
      }
      continue;
    }

    if (ent.isFile()) {
      if (isTsFile(full) && !isSpecTsFile(full) && !isExcludedPath(full, excludes)) {
        results.push(full);
      }
    }
  }

  return results;
};

const uniqSorted = (arr) => {
  const set = new Set(arr.map((p) => path.normalize(p)));
  return Array.from(set).sort((a, b) => a.localeCompare(b));
};

const writeCombinedOutput = async ({ files, outFile, showRelative }) => {
  // overwrite
  const stream = fs.createWriteStream(outFile, { flags: 'w', encoding: 'utf8' });

  const write = (s) =>
    new Promise((resolve, reject) => {
      stream.write(s, (err) => (err ? reject(err) : resolve()));
    });

  const close = () =>
    new Promise((resolve, reject) => {
      stream.end(() => resolve());
      stream.on('error', reject);
    });

  const cwd = process.cwd();

  for (const filePath of files) {
    const displayName = showRelative ? path.relative(cwd, filePath) : path.basename(filePath);

    await write(`archivo: ${displayName}\r\n${FENCE}typescript\n`);

    const content = await fsp.readFile(filePath, 'utf8');
    await write(content);

    if (!content.endsWith('\n') && !content.endsWith('\r\n')) {
      await write('\n');
    }

    await write(`${FENCE}\n`);
  }

  await close();
};

const parseArgs = (argv) => {
  const args = argv.slice(2);
  const opts = {
    outFile: DEFAULT_OUT,
    excludes: new Set(DEFAULT_EXCLUDES),
    pick: false,
    showRelative: false,
    paths: [],
    help: false,
  };

  for (let i = 0; i < args.length; i++) {
    const a = args[i];

    if (a === '-h' || a === '--help') {
      opts.help = true;
      continue;
    }

    if (a === '-o' || a === '--out') {
      opts.outFile = args[i + 1] || DEFAULT_OUT;
      i++;
      continue;
    }

    if (a === '--exclude') {
      const raw = args[i + 1] || '';
      i++;
      raw
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)
        .forEach((name) => opts.excludes.add(name));
      continue;
    }

    if (a === '--pick') {
      opts.pick = true;
      continue;
    }

    if (a === '--relative') {
      opts.showRelative = true;
      continue;
    }

    opts.paths.push(a);
  }

  return opts;
};

const printHelp = () => {
  console.log(`
Uso:
  node combine-ts.js [rutas...] [opciones]

Si NO pasas rutas:
  - combina desde el directorio actual (recursivo), ignorando node_modules/ y dist/

Si pasas rutas:
  - cada ruta puede ser una carpeta (recursivo) o un archivo .ts

Opciones:
  -o, --out <archivo>       Output (default: ${DEFAULT_OUT})
  --exclude <a,b,c>         Agrega nombres de carpetas a excluir (default: node_modules,dist)
  --relative                En 'archivo:' escribe ruta relativa (en vez del nombre del archivo)
  --pick                    Modo interactivo (elige items del directorio actual)
  -h, --help                Ayuda

Ejemplos:
  node combine-ts.js
  node combine-ts.js src shared/types.ts
  node combine-ts.js packages/app packages/lib --relative -o combined.txt
  node combine-ts.js --pick
`.trim());
};

const listPickCandidates = async (baseDir, excludes) => {
  const entries = await fsp.readdir(baseDir, { withFileTypes: true });
  const items = [];

  for (const ent of entries) {
    const full = path.join(baseDir, ent.name);

    if (ent.isDirectory()) {
      if (excludes.has(ent.name)) continue;
      items.push({ type: 'dir', name: ent.name, full });
      continue;
    }

    if (ent.isFile() && isTsFile(ent.name) && !isSpecTsFile(ent.name)) {
      items.push({ type: 'file', name: ent.name, full });
    }
  }

  // dirs primero, luego archivos
  items.sort((a, b) => {
    if (a.type !== b.type) return a.type === 'dir' ? -1 : 1;
    return a.name.localeCompare(b.name);
  });

  return items;
};

const parsePickSelection = (input, maxIndex) => {
  const raw = (input || '').trim();
  if (!raw) return null; // null = todo

  const chosen = new Set();
  const parts = raw.split(',').map((s) => s.trim()).filter(Boolean);

  for (const part of parts) {
    if (part.includes('-')) {
      const [a, b] = part.split('-').map((x) => parseInt(x.trim(), 10));
      if (Number.isFinite(a) && Number.isFinite(b)) {
        const start = Math.min(a, b);
        const end = Math.max(a, b);
        for (let i = start; i <= end; i++) {
          if (i >= 1 && i <= maxIndex) chosen.add(i);
        }
      }
    } else {
      const n = parseInt(part, 10);
      if (Number.isFinite(n) && n >= 1 && n <= maxIndex) chosen.add(n);
    }
  }

  return Array.from(chosen).sort((x, y) => x - y);
};

const ask = async (question) => {
  const rl = readline.createInterface({ input: process.stdin, output: process.stdout });
  const answer = await new Promise((resolve) => rl.question(question, resolve));
  rl.close();
  return answer;
};

const main = async () => {
  const opts = parseArgs(process.argv);

  if (opts.help) {
    printHelp();
    process.exit(0);
  }

  let roots = opts.paths.length ? opts.paths : ['.'];

  if (opts.pick && (!opts.paths || opts.paths.length === 0)) {
    const baseDir = process.cwd();
    const items = await listPickCandidates(baseDir, opts.excludes);

    if (items.length === 0) {
      console.log('No encontre carpetas/archivos candidatos en el directorio actual.');
      process.exit(1);
    }

    console.log('\nElegí qué incluir (items del directorio actual):');
    items.forEach((it, idx) => {
      console.log(`  ${idx + 1}. [${it.type}] ${it.name}`);
    });

    const input = await ask('\nIngresá indices (ej: 1,3,5-7) o Enter para TODO: ');
    const sel = parsePickSelection(input, items.length);

    if (sel === null) {
      roots = ['.'];
    } else {
      roots = sel.map((i) => items[i - 1].full);
    }
  }

  const allFiles = [];
  for (const r of roots) {
    const abs = path.resolve(process.cwd(), r);
    const files = await walkForTsFiles(abs, opts.excludes);
    allFiles.push(...files);
  }

  const files = uniqSorted(allFiles);

  await writeCombinedOutput({
    files,
    outFile: path.resolve(process.cwd(), opts.outFile),
    showRelative: opts.showRelative,
  });

  console.log(`Listo. Archivos combinados: ${files.length}`);
  console.log(`Output: ${opts.outFile}`);
};

main().catch((err) => {
  console.error('Error:', err && err.stack ? err.stack : err);
  process.exit(1);
});
