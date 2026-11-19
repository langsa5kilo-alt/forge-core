
// Entry launcher for the built forge: resolve the binary matching this host's arch,
// hand it our stdio and environment, mirror its signals, and exit as it exits.
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';

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
