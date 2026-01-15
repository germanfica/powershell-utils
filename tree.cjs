#!/usr/bin/env node
'use strict';

const fs = require('fs');
const fsp = fs.promises;
const path = require('path');

async function* walkFiles(rootDir) {
  const stack = [rootDir];

  while (stack.length) {
    const dir = stack.pop();

    let dh;
    try {
      dh = await fsp.opendir(dir);
    } catch {
      // Si no se puede abrir (permisos, etc.), lo salteamos
      continue;
    }

    for await (const dirent of dh) {
      const full = path.resolve(dir, dirent.name);

      // No seguir symlinks para evitar loops
      if (dirent.isSymbolicLink && dirent.isSymbolicLink()) continue;

      if (dirent.isDirectory()) {
        stack.push(full);
      } else if (dirent.isFile()) {
        yield full;
      } else {
        // Otros tipos (socket, fifo, etc.) se ignoran
      }
    }
  }
}

async function main() {
  // Uso:
  //   node list-files.js [directorio] [archivo_salida_opcional]
  const targetDir = path.resolve(process.argv[2] || process.cwd());
  const outFile = process.argv[3] ? path.resolve(process.argv[3]) : null;

  const out = outFile ? fs.createWriteStream(outFile, { encoding: 'utf8' }) : null;

  try {
    for await (const filePath of walkFiles(targetDir)) {
      const line = filePath + '\n';
      if (out) out.write(line);
      else process.stdout.write(line);
    }
  } finally {
    if (out) out.end();
  }
}

main().catch((err) => {
  console.error(err && err.stack ? err.stack : String(err));
  process.exit(1);
});