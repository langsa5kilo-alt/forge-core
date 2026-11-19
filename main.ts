
// Entry launcher for the built forge: resolve the binary matching this host's arch,
// hand it our stdio and environment, mirror its signals, and exit as it exits.
import { spawn } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';


// Go 1.23 does not read the cgroup CPU quota, so on a capped container it starts
// one runnable thread per host core and thrashes. Match the quota when one exists.
try {
  const [quota, period] = readFileSync('/sys/fs/cgroup/cpu.max', 'utf8').trim().split(/\s+/);
  if (quota !== 'max') process.env.GOMAXPROCS = String(Math.max(1, Math.floor(Number(quota) / Number(period))));
} catch {}

const target = process.arch === 'x64' ? 'forge-amd64' : 'forge-arm64';
const forge = fileURLToPath(new URL(`./${target}`, import.meta.url));
const child = spawn(forge, [], { env: process.env, stdio: 'inherit' });

for (const signal of ['SIGTERM', 'SIGINT', 'SIGHUP'] as const) {
  process.on(signal, () => {
    if (child.exitCode === null && child.signalCode === null) child.kill(signal);
  });
}

child.on('error', (error) => { console.error(error); process.exit(1); });
child.on('exit', (code, signal) => process.exit(code ?? (signal ? 1 : 0)));
