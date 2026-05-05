import { access, readdir, readFile } from 'node:fs/promises';
import path from 'node:path';
import { constants } from 'node:fs';

const root = path.resolve('skills');
const cliRoot = path.resolve('skills/cli');
const versionFile = path.resolve('internal/version/VERSION');
const packageFile = path.resolve('package.json');
const lockFile = path.resolve('package-lock.json');
const changelogFile = path.resolve('CHANGELOG.md');

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

async function verifyVersionParity() {
  const version = (await readFile(versionFile, 'utf8')).trim();
  if (!/^\d+\.\d+\.\d+(?:-[\w.]+)?$/.test(version)) {
    throw new Error(`internal/version/VERSION is not a valid semver: ${version}`);
  }

  const pkg = JSON.parse(await readFile(packageFile, 'utf8'));
  if (pkg.version !== version) {
    throw new Error(
      `package.json version (${pkg.version}) does not match internal/version/VERSION (${version}). Run: npm run sync:version`,
    );
  }

  const lock = JSON.parse(await readFile(lockFile, 'utf8'));
  const lockTopVersion = lock.version;
  const lockSelfVersion = lock.packages?.['']?.version;
  if (lockTopVersion !== version || lockSelfVersion !== version) {
    throw new Error(
      `package-lock.json versions (${lockTopVersion}, ${lockSelfVersion}) do not match internal/version/VERSION (${version}). Run: npm run sync:version`,
    );
  }

  const changelog = await readFile(changelogFile, 'utf8');
  const heading = `## v${version}`;
  if (!changelog.includes(heading)) {
    throw new Error(
      `CHANGELOG.md is missing a "${heading}" section. Add release notes before publishing ${version}.`,
    );
  }

  console.log(`Verified version parity at ${version} (VERSION, package.json, package-lock.json, CHANGELOG.md).`);
}

async function verify() {
  await verifyVersionParity();

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
