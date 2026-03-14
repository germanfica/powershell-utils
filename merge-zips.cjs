#!/usr/bin/env node
'use strict';

const fs = require('fs');
const path = require('path');
const os = require('os');
const { spawn } = require('child_process');

const { promises: fsp } = fs;

const printHelp = () => {
  console.log(`
Uso:
  node merge-zips.cjs --dir <carpeta> --out <salida> [--prefix <texto>] [--conflict overwrite|skip|rename|error] [--no-recursive] [--keep-temp] [--keep-zips]

Ejemplos:
  node merge-zips.cjs --dir . --out ./merged
  node merge-zips.cjs --dir ./downloads --prefix my-zip-file-name --out ./merged
  node merge-zips.cjs --dir ./downloads --out ./merged --conflict rename
`);
};

const parseArgs = (argv) => {
  const args = {
    dir: process.cwd(),
    out: path.resolve(process.cwd(), 'merged'),
    prefix: '',
    recursive: true,
    conflict: 'overwrite',
    keepTemp: false,
    deleteZips: true,
    help: false,
  };

  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];

    if (arg === '--dir') args.dir = path.resolve(argv[++i]);
    else if (arg === '--out') args.out = path.resolve(argv[++i]);
    else if (arg === '--prefix') args.prefix = argv[++i] || '';
    else if (arg === '--conflict') args.conflict = argv[++i] || 'overwrite';
    else if (arg === '--no-recursive') args.recursive = false;
    else if (arg === '--keep-temp') args.keepTemp = true;
    else if (arg === '--keep-zips') args.deleteZips = false;
    else if (arg === '--help' || arg === '-h') args.help = true;
    else throw new Error(`Argumento no reconocido: ${arg}`);
  }

  const allowed = new Set(['overwrite', 'skip', 'rename', 'error']);
  if (!allowed.has(args.conflict)) {
    throw new Error(`Valor inválido para --conflict: ${args.conflict}`);
  }

  return args;
};

const exists = async (targetPath) => {
  try {
    await fsp.access(targetPath, fs.constants.F_OK);
    return true;
  } catch {
    return false;
  }
};

const ensureDir = async (dirPath) => {
  await fsp.mkdir(dirPath, { recursive: true });
};

const run = (command, args, options = {}) =>
  new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      stdio: 'inherit',
      ...options,
    });

    child.on('error', reject);
    child.on('close', (code) => {
      if (code === 0) resolve();
      else reject(new Error(`${command} terminó con código ${code}`));
    });
  });

const checkUnzipAvailable = async () => {
  try {
    await run('unzip', ['-v'], { stdio: 'ignore' });
  } catch {
    throw new Error(
      'No encontré el comando "unzip". En Ubuntu instalalo con: sudo apt install unzip'
    );
  }
};

const walk = async (dirPath, recursive = true) => {
  const entries = await fsp.readdir(dirPath, { withFileTypes: true });
  const results = [];

  for (const entry of entries) {
    const fullPath = path.join(dirPath, entry.name);

    if (entry.isDirectory()) {
      if (recursive) {
        const nested = await walk(fullPath, recursive);
        results.push(...nested);
      }
      continue;
    }

    if (entry.isFile() && entry.name.toLowerCase().endsWith('.zip')) {
      results.push(fullPath);
    }
  }

  return results;
};

const getFamilyInfo = (zipPath) => {
  const fileName = path.basename(zipPath);
  const match = fileName.match(/^(.*)-(\d{3,})\.zip$/i);

  if (!match) return null;

  return {
    fileName,
    family: match[1],
    part: Number(match[2]),
  };
};

const groupZipFiles = (zipFiles, prefix = '') => {
  const groups = new Map();

  for (const zipPath of zipFiles) {
    const info = getFamilyInfo(zipPath);
    if (!info) continue;
    if (prefix && !info.family.includes(prefix)) continue;

    if (!groups.has(info.family)) groups.set(info.family, []);

    groups.get(info.family).push({
      zipPath,
      fileName: info.fileName,
      family: info.family,
      part: info.part,
    });
  }

  for (const files of groups.values()) {
    files.sort((a, b) => a.part - b.part || a.fileName.localeCompare(b.fileName));
  }

  return groups;
};

const toSafeName = (value) => value.replace(/[<>:"/\\|?*\x00-\x1F]/g, '_');

const getUniquePath = async (targetPath) => {
  if (!(await exists(targetPath))) return targetPath;

  const parsed = path.parse(targetPath);
  let counter = 2;

  while (true) {
    const candidate = path.join(parsed.dir, `${parsed.name} (${counter})${parsed.ext}`);
    if (!(await exists(candidate))) return candidate;
    counter += 1;
  }
};

const mergeDirectory = async (sourceDir, destDir, conflictMode) => {
  await ensureDir(destDir);
  const entries = await fsp.readdir(sourceDir, { withFileTypes: true });

  for (const entry of entries) {
    const srcPath = path.join(sourceDir, entry.name);
    const destPath = path.join(destDir, entry.name);

    if (entry.isDirectory()) {
      await mergeDirectory(srcPath, destPath, conflictMode);
      continue;
    }

    if (!entry.isFile()) continue;

    const alreadyExists = await exists(destPath);

    if (!alreadyExists) {
      await ensureDir(path.dirname(destPath));
      await fsp.copyFile(srcPath, destPath);
      continue;
    }

    if (conflictMode === 'skip') continue;

    if (conflictMode === 'error') {
      throw new Error(`Conflicto de archivo: ${destPath}`);
    }

    if (conflictMode === 'rename') {
      const renamedPath = await getUniquePath(destPath);
      await ensureDir(path.dirname(renamedPath));
      await fsp.copyFile(srcPath, renamedPath);
      continue;
    }

    await ensureDir(path.dirname(destPath));
    await fsp.copyFile(srcPath, destPath);
  }
};

const extractZip = async (zipPath, destDir) => {
  await ensureDir(destDir);
  await run('unzip', ['-qq', '-o', zipPath, '-d', destDir]);
};

const deleteZip = async (zipPath) => {
  await fsp.unlink(zipPath);
};

const processFamily = async (family, files, options, singleFamily) => {
  const safeFamily = toSafeName(family);
  const finalDir = singleFamily ? options.out : path.join(options.out, safeFamily);
  const tempRoot = path.join(
    os.tmpdir(),
    `merge-zips-${safeFamily}-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
  );

  await ensureDir(finalDir);
  await ensureDir(tempRoot);

  console.log(`\nFamilia: ${family}`);
  console.log(`Salida: ${finalDir}`);

  try {
    for (let i = 0; i < files.length; i += 1) {
      const item = files[i];
      const tempExtractDir = path.join(tempRoot, String(item.part).padStart(3, '0'));

      console.log(`[${i + 1}/${files.length}] Extrayendo ${item.fileName}`);
      await extractZip(item.zipPath, tempExtractDir);

      console.log(`[${i + 1}/${files.length}] Merge ${item.fileName}`);
      await mergeDirectory(tempExtractDir, finalDir, options.conflict);

      if (options.deleteZips) {
        console.log(`[${i + 1}/${files.length}] Borrando ${item.fileName}`);
        await deleteZip(item.zipPath);
      }
    }
  } finally {
    if (!options.keepTemp) {
      await fsp.rm(tempRoot, { recursive: true, force: true });
    } else {
      console.log(`Temporal conservado en: ${tempRoot}`);
    }
  }

  return finalDir;
};

const main = async () => {
  const options = parseArgs(process.argv.slice(2));

  if (options.help) {
    printHelp();
    return;
  }

  await checkUnzipAvailable();

  if (!(await exists(options.dir))) {
    throw new Error(`La carpeta no existe: ${options.dir}`);
  }

  await ensureDir(options.out);

  console.log(`Buscando .zip en: ${options.dir}`);
  const zipFiles = await walk(options.dir, options.recursive);
  const groups = groupZipFiles(zipFiles, options.prefix);

  if (groups.size === 0) {
    console.log('No se encontraron archivos con patrón ...-NNN.zip');
    return;
  }

  const families = [...groups.keys()].sort((a, b) => a.localeCompare(b));
  const singleFamily = families.length === 1;

  console.log(`Se encontraron ${families.length} familia(s):`);
  for (const family of families) {
    const files = groups.get(family);
    const parts = files.map((x) => String(x.part).padStart(3, '0')).join(', ');
    console.log(`- ${family} -> ${parts}`);
  }

  const outputs = [];
  for (const family of families) {
    const outputDir = await processFamily(family, groups.get(family), options, singleFamily);
    outputs.push(outputDir);
  }

  console.log('\nListo.');
  outputs.forEach((dir) => console.log(`- ${dir}`));
};

main().catch((error) => {
  console.error(`\nError: ${error.message}`);
  process.exit(1);
});