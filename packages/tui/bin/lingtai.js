#!/usr/bin/env node

const { execFileSync } = require('child_process');
const os = require('os');
const path = require('path');

const platformKey = `${os.platform()}-${os.arch() === 'x64' ? 'x64' : os.arch()}`;

const packageMap = {
  'darwin-arm64': '@lingtai/tui-darwin-arm64',
  'darwin-x64': '@lingtai/tui-darwin-x64',
  'linux-x64': '@lingtai/tui-linux-x64',
  'linux-arm64': '@lingtai/tui-linux-arm64',
  'win32-x64': '@lingtai/tui-win32-x64',
};

const pkg = packageMap[platformKey];
if (!pkg) {
  console.error(`Unsupported platform: ${platformKey}`);
  process.exit(1);
}

let binPath;
try {
  const pkgDir = path.dirname(require.resolve(`${pkg}/package.json`));
  const binName = process.platform === 'win32' ? 'lingtai.exe' : 'lingtai';
  binPath = path.join(pkgDir, 'bin', binName);
} catch {
  console.error(`Platform package ${pkg} not installed. Try: npm install`);
  process.exit(1);
}

try {
  execFileSync(binPath, process.argv.slice(2), { stdio: 'inherit' });
} catch (e) {
  process.exit(e.status || 1);
}
