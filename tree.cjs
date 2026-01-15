#!/usr/bin/env node
"use strict";

const fs = require("fs");
const fsp = fs.promises;
const path = require("path");

const DEFAULT_EXCLUDES = new Set(["node_modules", "dist", ".git"]);

const normalizeCsv = (raw) =>
  String(raw || "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);

const isExcludedPath = (p, excludes) => {
  const normalized = path.normalize(p);
  const parts = normalized.split(path.sep).filter(Boolean);
  return parts.some((part) => excludes.has(part));
};

const statSafe = async (p) => {
  try {
    return await fsp.stat(p);
  } catch {
    return null;
  }
};

const readDirSafe = async (dir) => {
  try {
    return await fsp.readdir(dir, { withFileTypes: true });
  } catch {
    return null;
  }
};

const sortTreeEntries = (entries) =>
  entries.sort((a, b) => {
    const ad = a.isDirectory();
    const bd = b.isDirectory();
    if (ad !== bd) return ad ? -1 : 1; // dirs primero
    return a.name.localeCompare(b.name);
  });

const parseArgs = (argv) => {
  const args = argv.slice(2);

  const opts = {
    target: ".", // positional
    outFile: null,
    excludes: new Set(DEFAULT_EXCLUDES),
    maxDepth: Infinity,
    relative: false,
    listMode: false, // --list => una ruta por linea
    filesOnly: false,
    dirsOnly: false,
    help: false,
  };

  const positionals = [];

  for (let i = 0; i < args.length; i++) {
    const a = args[i];

    if (a === "-h" || a === "--help") {
      opts.help = true;
      continue;
    }

    if (a === "-o" || a === "--out") {
      opts.outFile = args[i + 1] ? String(args[i + 1]) : null;
      i++;
      continue;
    }

    if (a === "--exclude") {
      const raw = args[i + 1] || "";
      i++;
      normalizeCsv(raw).forEach((name) => opts.excludes.add(name));
      continue;
    }

    if (a === "--max-depth") {
      const n = Number(args[i + 1]);
      i++;
      if (Number.isFinite(n) && n >= 0) opts.maxDepth = n;
      continue;
    }

    if (a === "--relative") {
      opts.relative = true;
      continue;
    }

    if (a === "--list") {
      opts.listMode = true;
      continue;
    }

    if (a === "--files-only") {
      opts.filesOnly = true;
      continue;
    }

    if (a === "--dirs-only") {
      opts.dirsOnly = true;
      continue;
    }

    positionals.push(a);
  }

  if (positionals.length) opts.target = positionals[0];

  // Si pasan ambos, priorizamos dirs-only (o podés cambiarlo)
  if (opts.filesOnly && opts.dirsOnly) {
    opts.filesOnly = false;
  }

  return opts;
};

const printHelp = () => {
  console.log(
    `
Uso:
  node tree.cjs [ruta] [opciones]

Opciones:
  -o, --out <archivo>       Escribe el output a un archivo (si no, stdout)
  --exclude <a,b,c>         Excluye carpetas por nombre (repetible)
                            (default: ${Array.from(DEFAULT_EXCLUDES).join(",")})
  --max-depth <n>           Profundidad maxima (0 = solo raiz)
  --relative                Muestra paths relativos al cwd en --list y nombre raiz
  --list                    Modo lista: una ruta por linea (en vez de arbol)
  --files-only              Solo archivos (arbol/lista)
  --dirs-only               Solo directorios (arbol/lista)
  -h, --help                Ayuda

Ejemplos:
  node tree.cjs
  node tree.cjs . --exclude node_modules,dist
  node tree.cjs src --exclude node_modules --exclude dist --max-depth 4
  node tree.cjs . --list --relative --exclude node_modules,dist -o tree.txt
`.trim()
  );
};

const makeWriter = (outFile) => {
  if (!outFile) {
    return {
      writeLine: async (s) => process.stdout.write(s + "\n"),
      close: async () => { },
    };
  }

  const stream = fs.createWriteStream(outFile, { flags: "w", encoding: "utf8" });

  const writeLine = (s) =>
    new Promise((resolve, reject) => {
      stream.write(s + "\n", (err) => (err ? reject(err) : resolve()));
    });

  const close = () =>
    new Promise((resolve, reject) => {
      stream.end(() => resolve());
      stream.on("error", reject);
    });

  return { writeLine, close };
};

const shouldKeepEntry = (fullPath, ent, opts) => {
  // no seguir symlinks (evita loops)
  if (ent.isSymbolicLink && ent.isSymbolicLink()) return false;

  // excluir por path (si en cualquier segmento aparece el nombre excluido)
  if (isExcludedPath(fullPath, opts.excludes)) return false;

  if (opts.filesOnly) return ent.isFile();
  if (opts.dirsOnly) return ent.isDirectory();

  // default: ambos
  return ent.isDirectory() || ent.isFile();
};

// Modo lista: imprime paths (archivos y/o dirs) respetando exclude y maxDepth
const walkList = async (rootDir, opts, writeLine) => {
  const rootStat = await statSafe(rootDir);
  if (!rootStat) return;

  const display = (p) => {
    if (!opts.relative) return p;
    const rel = path.relative(process.cwd(), p);
    return rel || ".";
  };

  // si el root es archivo
  if (rootStat.isFile()) {
    if (!opts.dirsOnly) await writeLine(display(rootDir));
    return;
  }

  if (!rootStat.isDirectory()) return;

  const stack = [{ dir: rootDir, depth: 0 }];

  while (stack.length) {
    const { dir, depth } = stack.pop();
    const entries = await readDirSafe(dir);
    if (!entries) continue;

    for (const ent of entries) {
      const full = path.join(dir, ent.name);
      if (!shouldKeepEntry(full, ent, opts)) continue;

      if (ent.isDirectory()) {
        if (!opts.filesOnly) await writeLine(display(full));
        if (depth + 1 <= opts.maxDepth) stack.push({ dir: full, depth: depth + 1 });
      } else if (ent.isFile()) {
        if (!opts.dirsOnly) await writeLine(display(full));
      }
    }
  }
};

// Modo arbol: imprime un "tree" ASCII, sin recursion (stack de frames)
const walkTree = async (rootDir, opts, writeLine) => {
  const rootStat = await statSafe(rootDir);
  if (!rootStat) return;

  const displayRoot = () => {
    if (!opts.relative) return path.basename(rootDir) || rootDir;
    const rel = path.relative(process.cwd(), rootDir);
    return rel || ".";
  };

  // si el root es archivo
  if (rootStat.isFile()) {
    if (!opts.dirsOnly) await writeLine(displayRoot());
    return;
  }

  if (!rootStat.isDirectory()) return;

  await writeLine(displayRoot());

  const getFilteredEntries = async (dir) => {
    const entries = await readDirSafe(dir);
    if (!entries) return [];

    const filtered = [];
    for (const ent of entries) {
      const full = path.join(dir, ent.name);
      if (!shouldKeepEntry(full, ent, opts)) continue;
      filtered.push(ent);
    }

    return sortTreeEntries(filtered);
  };

  const frames = [
    {
      dir: rootDir,
      prefix: "",
      depth: 0,
      entries: await getFilteredEntries(rootDir),
      idx: 0,
    },
  ];

  while (frames.length) {
    const frame = frames[frames.length - 1];

    if (frame.idx >= frame.entries.length) {
      frames.pop();
      continue;
    }

    const ent = frame.entries[frame.idx];
    frame.idx++;

    const isLast = frame.idx === frame.entries.length;
    const connector = isLast ? "└── " : "├── ";
    const childPrefix = frame.prefix + (isLast ? "    " : "│   ");

    const full = path.join(frame.dir, ent.name);
    const label = ent.isDirectory() ? `${ent.name}/` : ent.name;

    await writeLine(frame.prefix + connector + label);

    if (ent.isDirectory() && frame.depth + 1 <= opts.maxDepth) {
      frames.push({
        dir: full,
        prefix: childPrefix,
        depth: frame.depth + 1,
        entries: await getFilteredEntries(full),
        idx: 0,
      });
    }
  }
};

const main = async () => {
  const opts = parseArgs(process.argv);

  if (opts.help) {
    printHelp();
    process.exit(0);
  }

  const targetAbs = path.resolve(process.cwd(), opts.target);
  const outFileAbs = opts.outFile ? path.resolve(process.cwd(), opts.outFile) : null;

  const { writeLine, close } = makeWriter(outFileAbs);

  try {
    if (opts.listMode) {
      await walkList(targetAbs, opts, writeLine);
    } else {
      await walkTree(targetAbs, opts, writeLine);
    }
  } finally {
    await close();
  }
};

main().catch((err) => {
  console.error(err && err.stack ? err.stack : String(err));
  process.exit(1);
});
