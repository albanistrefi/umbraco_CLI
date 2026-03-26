import { access, readdir } from 'node:fs/promises';
import path from 'node:path';
import { constants } from 'node:fs';

const root = path.resolve('skills');

async function collectSkillDirs(baseDir) {
  const firstLevel = await readdir(baseDir, { withFileTypes: true });
  const skillPaths = [];

  for (const categoryEntry of firstLevel) {
    if (!categoryEntry.isDirectory()) {
      continue;
    }

    const categoryPath = path.join(baseDir, categoryEntry.name);
    const maybeSkills = await readdir(categoryPath, { withFileTypes: true });

    for (const skillEntry of maybeSkills) {
      if (!skillEntry.isDirectory()) {
        continue;
      }

      skillPaths.push(path.join(categoryPath, skillEntry.name));
    }
  }

  return skillPaths.sort();
}

async function verify() {
  const skillDirs = await collectSkillDirs(root);

  if (skillDirs.length !== 67) {
    throw new Error(`Expected 67 bundled skills but found ${skillDirs.length}`);
  }

  for (const skillDir of skillDirs) {
    const skillFile = path.join(skillDir, 'SKILL.md');
    await access(skillFile, constants.R_OK);
  }

  console.log(`Verified ${skillDirs.length} bundled skills and SKILL.md presence.`);
}

verify().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
});
