import { cp, mkdir, readdir, rm, stat } from 'node:fs/promises';
import path from 'node:path';
import { EXCLUDED_SKILLS, resolveCategory } from './skills-manifest.mjs';

const sourceRoot = path.resolve('.agents/skills');
const destinationRoot = path.resolve('skills');

function isUmbracoSkill(entry) {
  return entry.name.startsWith('umbraco-');
}

async function listSourceSkills() {
  const entries = await readdir(sourceRoot, { withFileTypes: true });
  const skills = entries
    .filter((entry) => entry.isDirectory() && isUmbracoSkill(entry) && !EXCLUDED_SKILLS.has(entry.name))
    .map((entry) => entry.name)
    .sort();

  if (skills.length !== 67) {
    throw new Error(`Expected to bundle 67 skills, found ${skills.length}.`);
  }

  return skills;
}

async function ensureSourceSkillIsValid(skillName) {
  const skillPath = path.join(sourceRoot, skillName);
  const skillStat = await stat(skillPath);
  if (!skillStat.isDirectory()) {
    throw new Error(`Skill path is not a directory: ${skillPath}`);
  }
}

async function bundle() {
  const skills = await listSourceSkills();

  await rm(destinationRoot, { recursive: true, force: true });
  await mkdir(destinationRoot, { recursive: true });

  for (const skillName of skills) {
    await ensureSourceSkillIsValid(skillName);

    const category = resolveCategory(skillName);
    const targetDir = path.join(destinationRoot, category, skillName);

    await mkdir(path.dirname(targetDir), { recursive: true });
    await cp(path.join(sourceRoot, skillName), targetDir, {
      recursive: true,
      force: true,
    });
  }

  console.log(`Bundled ${skills.length} Umbraco skills into ${destinationRoot}`);
}

bundle().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
});
