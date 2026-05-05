// Reads internal/version/VERSION and writes the same value into package.json and package-lock.json.
// Run via `npm run sync:version` after editing the VERSION file.

import { readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';

const repoRoot = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..');
const versionFile = path.join(repoRoot, 'internal', 'version', 'VERSION');
const packageFile = path.join(repoRoot, 'package.json');
const lockFile = path.join(repoRoot, 'package-lock.json');

function trim(value) {
  return value.toString().trim();
}

async function readJson(file) {
  const raw = await readFile(file, 'utf8');
  return { raw, parsed: JSON.parse(raw) };
}

async function writeJson(file, data) {
  await writeFile(file, JSON.stringify(data, null, 2) + '\n', 'utf8');
}

async function main() {
  const version = trim(await readFile(versionFile, 'utf8'));
  if (!/^\d+\.\d+\.\d+(?:-[\w.]+)?$/.test(version)) {
    throw new Error(`internal/version/VERSION does not contain a valid semver: ${version}`);
  }

  const { parsed: pkg } = await readJson(packageFile);
  pkg.version = version;
  await writeJson(packageFile, pkg);

  const { parsed: lock } = await readJson(lockFile);
  lock.version = version;
  if (lock.packages && lock.packages['']) {
    lock.packages[''].version = version;
  }
  await writeJson(lockFile, lock);

  console.log(`Synced version ${version} into package.json and package-lock.json.`);
}

main().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
});
