#!/usr/bin/env node
const { spawn } = require('child_process');
const path = require('path');

// 平台映射
const PLATFORMS = {
  'win32-x64': '@codekanban/win32-x64',
  'darwin-x64': '@codekanban/darwin-x64',
  'darwin-arm64': '@codekanban/darwin-arm64',
  'linux-x64': '@codekanban/linux-x64',
  'linux-arm64': '@codekanban/linux-arm64',
};

const platform = process.platform;
const arch = process.arch;
const platformKey = `${platform}-${arch}`;
const packageName = PLATFORMS[platformKey];

if (!packageName) {
  console.error(`Unsupported platform: ${platformKey}`);
  console.error('Supported platforms:', Object.keys(PLATFORMS).join(', '));
  process.exit(1);
}

// 查找二进制文件
let binPath;
try {
  const packagePath = require.resolve(packageName + '/package.json');
  const packageDir = path.dirname(packagePath);
  const binName = platform === 'win32' ? 'codekanban.exe' : 'codekanban';
  binPath = path.join(packageDir, binName);
} catch (e) {
  console.error(`Failed to find binary for ${platformKey}`);
  console.error(`Make sure ${packageName} is installed`);
  console.error('');
  console.error('Try one of the following:');
  console.error('  1. Uninstall and reinstall with npm:');
  console.error('     npm uninstall -g codekanban');
  console.error('     npm install -g codekanban');
  console.error('  2. Or uninstall and reinstall with pnpm:');
  console.error('     pnpm uninstall -g codekanban');
  console.error('     pnpm install -g codekanban');
  process.exit(1);
}

// 运行二进制
const child = spawn(binPath, process.argv.slice(2), {
  stdio: 'inherit',
  windowsHide: false
});

child.on('error', (err) => {
  console.error('Failed to start binary:', err.message);
  process.exit(1);
});

child.on('exit', (code) => {
  process.exit(code || 0);
});
