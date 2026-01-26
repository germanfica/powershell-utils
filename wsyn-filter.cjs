#!/usr/bin/env node
"use strict";

const { exec } = require("child_process");
const { promisify } = require("util");

const execAsync = promisify(exec);

const VERSION = "0.1.0";

const run = (cmd) =>
  execAsync(cmd, { windowsHide: true }).then((r) => r.stdout);

/* ---------------- utils ---------------- */

const csv = (v) =>
  String(v || "")
    .split(",")
    .map((s) => s.trim().toLowerCase())
    .filter(Boolean);

const uniq = (arr) => [...new Set(arr)];

const parseArgs = (argv) => {
  const opts = {
    include: [],
    exclude: [],
    help: false,
    version: false,
  };

  for (let i = 2; i < argv.length; i++) {
    const a = argv[i].toLowerCase();

    if (
      a === "help" ||
      a === "--help" ||
      a === "-h" ||
      a === "-help"
    ) {
      opts.help = true;
      continue;
    }

    if (
      a === "version" ||
      a === "--version" ||
      a === "-v" ||
      a === "-version"
    ) {
      opts.version = true;
      continue;
    }

    if (a === "--include") {
      opts.include = csv(argv[++i]);
      continue;
    }

    if (a === "--exclude") {
      opts.exclude = csv(argv[++i]);
      continue;
    }
  }

  return opts;
};

const printHelp = () => {
  console.log(`
Uso:
  wsyn-filter [opciones]

Opciones:
  --include a.exe,b.exe   Solo considera estos procesos
  --exclude a.exe,b.exe   Ignora estos procesos
  help, -h, --help        Muestra esta ayuda
  version, -v, --version Muestra la version

Ejemplos:
  wsyn-filter help
  wsyn-filter --exclude chrome.exe,spotify.exe
  wsyn-filter --include discord.exe
  wsyn-filter --include chrome.exe --exclude updater.exe

Salida:
  Display filter de Wireshark basado en:
    tcp.flags.syn == 1
`.trim());
};

/* ------------- netstat parsing ---------- */

const parseNetstat = (out) => {
  const rows = [];

  out.split("\n").forEach((l) => {
    const p = l.trim().split(/\s+/);
    if (p.length < 5) return;
    if (!/^\d+$/.test(p[4])) return;

    const [, , remote, , pid] = p;
    const [rip, rport] = remote.split(":");

    if (!rip || rip === "0.0.0.0" || rip === "*") return;

    rows.push({ pid, rip, rport });
  });

  return rows;
};

const getProcessName = async (pid) => {
  try {
    const out = await run(
      `wmic process where processid=${pid} get Name /value`
    );
    return out.match(/Name=(.+)/)?.[1]?.toLowerCase() || null;
  } catch {
    return null;
  }
};

/* ------------- filter builder ------------ */

const main = async () => {
  const opts = parseArgs(process.argv);

  if (opts.version) {
    console.log(VERSION);
    process.exit(0);
  }

  if (opts.help) {
    printHelp();
    process.exit(0);
  }

  const netstatOut = await run("netstat -ano");
  const conns = parseNetstat(netstatOut);

  const resolved = [];

  for (const c of conns) {
    const name = await getProcessName(c.pid);
    if (!name) continue;
    resolved.push({ ...c, name });
  }

  const filtered = resolved.filter((r) => {
    if (opts.include.length && !opts.include.includes(r.name)) return false;
    if (opts.exclude.includes(r.name)) return false;
    return true;
  });

  const ips = uniq(filtered.map((r) => r.rip));
  const ports = uniq(filtered.map((r) => r.rport));

  let filter = "tcp.flags.syn == 1";
  const clauses = [];

  if (ips.length)
    clauses.push(`ip.addr == ${ips.join(" or ip.addr == ")}`);

  if (ports.length)
    clauses.push(`tcp.port == ${ports.join(" or tcp.port == ")}`);

  if (clauses.length) {
    filter += " and not (" + clauses.join(" or ") + ")";
  }

  console.log("\nFiltro Wireshark generado:\n");
  console.log(filter);
};

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
