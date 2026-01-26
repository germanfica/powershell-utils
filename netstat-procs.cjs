const { exec } = require('child_process')
const { promisify } = require('util')

const execAsync = promisify(exec)

// Ejecuta comandos
const run = cmd =>
  execAsync(cmd, { windowsHide: true }).then(r => r.stdout)

// Extrae PIDs únicos desde netstat
const parseNetstat = output => {
  const pids = new Set()

  output.split('\n').forEach(line => {
    const parts = line.trim().split(/\s+/)
    const pid = parts[parts.length - 1]
    if (/^\d+$/.test(pid)) {
      pids.add(pid)
    }
  })

  return [...pids]
}

// PID → nombre + ruta
const getProcessInfo = pid =>
  run(
    `wmic process where processid=${pid} get Name,ExecutablePath /format:list`
  )
    .then(out => {
      const name = out.match(/Name=(.+)/)?.[1]
      const path = out.match(/ExecutablePath=(.+)/)?.[1]

      if (!name) return null
      return { pid, name, path }
    })
    .catch(() => null)

// Main
const main = async () => {
  console.log('Recolectando procesos con red activa...\n')

  const netstatOut = await run('netstat -ano')
  const pids = parseNetstat(netstatOut)

  const results = []

  for (const pid of pids) {
    const info = await getProcessInfo(pid)
    if (info) results.push(info)
  }

  console.table(results)
}

main().catch(console.error)
