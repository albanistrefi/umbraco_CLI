import { createHash } from 'node:crypto';
import { readFile, readdir } from 'node:fs/promises';
import path from 'node:path';

async function collectFiles(dir, base = dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  const files = [];

  for (const entry of entries) {
    const entryPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...(await collectFiles(entryPath, base)));
    } else if (entry.isFile()) {
      files.push(path.relative(base, entryPath));
    }
  }

  return files.sort();
}

// Hashes a skill directory as sha256 over "<relative path>\n<sha256(content)>\n"
// entries sorted by path, so the hash is stable across platforms and copy order.
export async function computeSkillHash(skillDir) {
  const hash = createHash('sha256');

  for (const relativePath of await collectFiles(skillDir)) {
    const content = await readFile(path.join(skillDir, relativePath));
    const contentHash = createHash('sha256').update(content).digest('hex');
    hash.update(`${relativePath.split(path.sep).join('/')}\n${contentHash}\n`);
  }

  return hash.digest('hex');
}
