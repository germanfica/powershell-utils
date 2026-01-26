#!/usr/bin/env node
"use strict";

const { exec } = require("child_process");
const { promisify } = require("util");

const execAsync = promisify(exec);

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
  };

  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (a === "--include") opts.include = csv(argv[++i]);
    if (a === "--exclude") opts.exclude = csv(argv[++i]);
  }

  return opts;
};

/* ------------- netstat parsing ---------- */

const parseNetstat = (out) => {
  const rows = [];

  out.split("\n").forEach((l) => {
    const p = l.trim().split(/\s+/);
    if (p.length < 5) return;
    if (!/^\d+$/.test(p[4])) return;

    const [proto, local, remote, , pid] = p;

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

main().catch(console.error);
