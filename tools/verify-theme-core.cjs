/* Usage: node tools/verify-theme-core.cjs <directory containing compiled core.js and __tests__/contracts.js> */
const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '..');
const compiled = process.argv[2];
if (!compiled) throw new Error('Pass the TypeScript output directory; see docs/ui/VALIDATION.md');
const { themeContractCases } = require(path.resolve(compiled, '__tests__/contracts.js'));
const builtin = JSON.parse(fs.readFileSync(path.join(root, 'ui/src/themes/builtin.json'), 'utf8'));
const result = { passed: [], failed: [] };
for (const test of themeContractCases(builtin)) {
  try { test.run(); result.passed.push(test.name); }
  catch (error) { result.failed.push({ name: test.name, error: String(error) }); }
}
fs.mkdirSync(path.join(root, 'docs/ui'), { recursive: true });
fs.writeFileSync(path.join(root, 'docs/ui/theme-contract-results.json'), JSON.stringify(result, null, 2) + '\n');
console.log(JSON.stringify({ passed: result.passed.length, failed: result.failed }, null, 2));
process.exitCode = result.failed.length ? 1 : 0;
