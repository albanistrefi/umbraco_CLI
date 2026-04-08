import { access, readdir } from 'node:fs/promises';
import path from 'node:path';
import { constants } from 'node:fs';

const root = path.resolve('skills');
const cliRoot = path.resolve('skills/cli');

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

async function collectCliSkillDirs(baseDir) {
  const entries = await readdir(baseDir, { withFileTypes: true });
  return entries
    .filter((e) => e.isDirectory())
    .map((e) => path.join(baseDir, e.name))
    .sort();
}

async function verify() {
  // Verify bundled extension-development skills
  const skillDirs = await collectSkillDirs(root);
  // Subtract the cli/ directory from the category count
  const extensionSkills = skillDirs.filter((d) => !d.startsWith(cliRoot));

  if (extensionSkills.length !== 67) {
    throw new Error(`Expected 67 bundled skills but found ${extensionSkills.length}`);
  }

  for (const skillDir of extensionSkills) {
    const skillFile = path.join(skillDir, 'SKILL.md');
    await access(skillFile, constants.R_OK);
  }

  console.log(`Verified ${extensionSkills.length} bundled skills and SKILL.md presence.`);

  // Verify CLI-generated skills
  const cliSkills = await collectCliSkillDirs(cliRoot);
  if (cliSkills.length === 0) {
    throw new Error('No CLI skills found in skills/cli/. Run: umbraco generate-skills');
  }

  for (const skillDir of cliSkills) {
    const skillFile = path.join(skillDir, 'SKILL.md');
    await access(skillFile, constants.R_OK);
  }

  console.log(`Verified ${cliSkills.length} CLI-generated skills and SKILL.md presence.`);
}

verify().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
});
