/** Run the real frontend checks with existing dependencies. Never installs packages. */
import { spawnSync } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const ui = path.join(root, 'ui');
const out = path.join(root, 'docs/ui');
mkdirSync(out, { recursive: true });
const report = { startedAt: new Date().toISOString(), platform: process.platform, node: process.version, files: {}, checks: [] };
const sources = ['ui/src/App.vue', 'ui/src/main.ts', 'ui/src/themes/core.ts', 'ui/src/themes/builtin.json', 'ui/src/themes/adapter.ts', 'ui/src/stores/appearance.ts', 'ui/src/stores/settings.ts', 'ui/src/styles/appearance-designs.css', 'ui/src/components/appearance/ThemeStudio.vue', 'ui/src/components/appearance/AppearanceShell.vue', 'ui/src/components/web-session/WebSessionSidebarRow.vue', 'ui/src/components/web-session/styles/webSessionSidebar.css', 'ui/src/components/web-session/__tests__/sessionSidebarAppearance.test.ts'];
for (const file of sources) {
  report.files[file] = createHash('sha256').update(readFileSync(path.join(root, file))).digest('hex');
}
const checks = [
  ['type-check', 'node_modules/vue-tsc/bin/vue-tsc.js', ['--build']],
  ['build-only', 'node_modules/vite/bin/vite.js', ['build']],
  ['test', 'node_modules/vitest/vitest.mjs', ['run']],
];
const save = () => writeFileSync(path.join(out, 'validation-runtime-results.json'), JSON.stringify(report, null, 2) + '\n');
save();
let failed = false;
for (const [name, entry, args] of checks) {
  const started = Date.now();
  if (!existsSync(path.join(ui, entry))) {
    failed = true;
    report.checks.push({ name, status: 'blocked', exitCode: null, reason: 'Existing dependency entry point not found: ' + entry });
    save();
    continue;
  }
  console.log('RUN', name);
  const result = spawnSync(process.execPath, [entry, ...args], { cwd: ui, encoding: 'utf8', timeout: 600000, maxBuffer: 32 * 1024 * 1024, windowsHide: true });
  const status = result.error ? 'blocked' : result.status === 0 ? 'passed' : 'failed';
  const log = (result.stdout || '') + (result.stderr || '') + (result.error ? '\n' + String(result.error) : '');
  writeFileSync(path.join(out, `validation-${name}.log`), log);
  report.checks.push({ name, command: ['node', entry, ...args], status, exitCode: result.status, signal: result.signal, durationMs: Date.now() - started, log: `validation-${name}.log` });
  failed ||= status !== 'passed';
  save();
  console.log(name, status, 'exit=', result.status);
}
report.finishedAt = new Date().toISOString();
save();
console.log(JSON.stringify(report, null, 2));
process.exitCode = failed ? 1 : 0;
