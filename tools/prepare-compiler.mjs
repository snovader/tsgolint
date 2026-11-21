import { spawnSync } from 'node:child_process';
import { copyFileSync, existsSync, mkdirSync, readdirSync, readFileSync, rmSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const compiler = resolve(root, '..', 'typescript');
const source = JSON.parse(readFileSync(join(root, 'compiler-source.json'), 'utf8'));

function run(command, args, cwd = root, capture = false) {
  const result = spawnSync(command, args, {
    cwd,
    encoding: 'utf8',
    stdio: capture ? 'pipe' : 'inherit',
    shell: false,
  });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(`${command} failed (${result.status}): ${result.stderr ?? ''}`);
  }
  return result.stdout?.trim() ?? '';
}

function prepareCheckout() {
  if (!/^[a-f0-9]{40}$/.test(source.commit ?? '')) {
    throw new Error('compiler-source.json must pin a complete compiler commit.');
  }
  if (!existsSync(compiler)) {
    run('git', ['clone', '--filter=blob:none', '--depth=1', '--no-checkout', source.repository, compiler]);
    run('git', ['sparse-checkout', 'init', '--cone'], compiler);
    run('git', ['sparse-checkout', 'set', 'tsc/internal', 'tsc/cmd', 'tsc/testdata/fixtures'], compiler);
  } else {
    const remote = run('git', ['remote', 'get-url', 'origin'], compiler, true);
    if (remote !== source.repository) {
      throw new Error(`Expected ${source.repository} in ${compiler}, found ${remote}.`);
    }
    if (run('git', ['status', '--porcelain'], compiler, true)) {
      throw new Error(`Compiler checkout has local changes: ${compiler}`);
    }
    if (run('git', ['rev-parse', 'HEAD'], compiler, true) === source.commit) return;
  }
  run('git', ['fetch', '--depth=1', 'origin', source.commit], compiler);
  run('git', ['checkout', '--detach', source.commit], compiler);
}

function synchronizeCollections() {
  const directory = join(compiler, 'tsc/internal/collections');
  const files = readdirSync(directory).filter(file => file.endsWith('.go') && !file.endsWith('_test.go'));
  const target = join(root, 'internal/collections');
  mkdirSync(target, { recursive: true });
  for (const file of readdirSync(target)) {
    if (file.endsWith('.go') && !files.includes(file)) rmSync(join(target, file));
  }
  for (const file of files) copyFileSync(join(directory, file), join(target, file));
  run('git', ['apply', '--check', join(root, 'patches/collections-json.patch')]);
  run('git', ['apply', join(root, 'patches/collections-json.patch')]);
}

prepareCheckout();
synchronizeCollections();
console.log(`Prepared TypeScript ${source.commit} in ${compiler}`);
