/* Syntax-only checks, NOT a substitute for vue-tsc or a production Vite build.
 * Usage: node tools/verify-source-syntax.cjs [path-to-typescript-package]
 */
const fs = require('node:fs');
const path = require('node:path');
const ts = require(process.argv[2] || 'typescript');
const root = path.resolve(__dirname, '..');
const files = [
  'ui/src/App.vue', 'ui/src/main.ts', 'ui/src/stores/settings.ts', 'ui/src/utils/settingsBackup.ts',
  'ui/src/components/common/ThemeSwitcher.vue', 'ui/src/views/LoginView.vue',
  'ui/src/stores/appearance.ts', 'ui/src/composables/useAppearanceRuntime.ts',
  ...fs.readdirSync(path.join(root, 'ui/src/themes')).filter(f => f.endsWith('.ts')).map(f => `ui/src/themes/${f}`),
  ...fs.readdirSync(path.join(root, 'ui/src/components/appearance')).filter(f => f.endsWith('.vue')).map(f => `ui/src/components/appearance/${f}`),
];
const report = { scope: 'TypeScript parser/transpiler only; does not type-check external libraries or compile Vue templates', checked: [], errors: [] };
for (const file of files) {
  const source = fs.readFileSync(path.join(root, file), 'utf8');
  const code = file.endsWith('.vue') ? source.match(/<script\b[^>]*>([\s\S]*?)<\/script>/)?.[1] : source;
  if (code == null) { report.errors.push({ file, message: 'Script block missing' }); continue; }
  const out = ts.transpileModule(code, { fileName: file.replace(/\.vue$/, '.ts'), reportDiagnostics: true,
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext, isolatedModules: true } });
  for (const d of out.diagnostics || []) if (d.category === ts.DiagnosticCategory.Error) report.errors.push({ file, message: ts.flattenDiagnosticMessageText(d.messageText, '\n') });
  report.checked.push(file);
}
fs.mkdirSync(path.join(root, 'docs/ui'), { recursive: true });
fs.writeFileSync(path.join(root, 'docs/ui/source-syntax-results.json'), JSON.stringify(report, null, 2) + '\n');
console.log(JSON.stringify({ checked: report.checked.length, errors: report.errors }));
process.exitCode = report.errors.length ? 1 : 0;
