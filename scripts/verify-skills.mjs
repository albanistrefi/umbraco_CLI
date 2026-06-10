import { access, readdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { constants } from 'node:fs';
import { computeSkillHash } from './skill-hash.mjs';
import { EXPECTED_SKILL_COUNT } from './skills-manifest.mjs';

const root = path.resolve('skills');
const cliRoot = path.resolve('skills/cli');
const versionFile = path.resolve('internal/version/VERSION');
const packageFile = path.resolve('package.json');
const lockFile = path.resolve('package-lock.json');
const changelogFile = path.resolve('CHANGELOG.md');
const skillsLockFile = path.resolve('skills-lock.json');

const updateLock = process.argv.includes('--update-lock');

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

async function loadSkillsLock() {
  const lock = JSON.parse(await readFile(skillsLockFile, 'utf8'));
  if (lock.version !== 1 || typeof lock.skills !== 'object' || lock.skills === null) {
    throw new Error('skills-lock.json has an unexpected shape; expected { version: 1, skills: {...} }.');
  }
  return lock;
}

async function verifySkillHashes(extensionSkillDirs) {
  const lock = await loadSkillsLock();
  const problems = [];
  const seen = new Set();

  for (const skillDir of extensionSkillDirs) {
    const name = path.basename(skillDir);
    seen.add(name);

    const entry = lock.skills[name];
    const computedHash = await computeSkillHash(skillDir);

    if (updateLock) {
      lock.skills[name] = {
        source: entry?.source ?? 'umbraco/Umbraco-CMS-Backoffice-Skills',
        sourceType: entry?.sourceType ?? 'github',
        computedHash,
      };
      continue;
    }

    if (!entry) {
      problems.push(`bundled skill "${name}" has no skills-lock.json entry`);
    } else if (entry.computedHash !== computedHash) {
      problems.push(`bundled skill "${name}" does not match its locked hash (content changed?)`);
    }
  }

  const stale = Object.keys(lock.skills).filter((name) => !seen.has(name));

  if (updateLock) {
    for (const name of stale) {
      delete lock.skills[name];
    }
    const sorted = Object.fromEntries(Object.entries(lock.skills).sort(([a], [b]) => a.localeCompare(b)));
    await writeFile(skillsLockFile, `${JSON.stringify({ version: 1, skills: sorted }, null, 2)}\n`);
    console.log(`Updated skills-lock.json with ${Object.keys(sorted).length} entries.`);
    return;
  }

  for (const name of stale) {
    problems.push(`skills-lock.json entry "${name}" has no bundled skill (stale entry?)`);
  }

  if (problems.length > 0) {
    throw new Error(
      `Skill hash verification failed:\n  - ${problems.join('\n  - ')}\n` +
        'If the changes are intentional, run: node scripts/verify-skills.mjs --update-lock',
    );
  }

  console.log(`Verified ${extensionSkillDirs.length} skill hashes against skills-lock.json.`);
}

async function verify() {
  await verifyVersionParity();

  // Verify bundled extension-development skills
  const skillDirs = await collectSkillDirs(root);
  // Subtract the cli/ directory from the category count
  const extensionSkills = skillDirs.filter((d) => !d.startsWith(cliRoot));

  if (extensionSkills.length !== EXPECTED_SKILL_COUNT) {
    throw new Error(`Expected ${EXPECTED_SKILL_COUNT} bundled skills but found ${extensionSkills.length}`);
  }

  for (const skillDir of extensionSkills) {
    const skillFile = path.join(skillDir, 'SKILL.md');
    await access(skillFile, constants.R_OK);
  }

  console.log(`Verified ${extensionSkills.length} bundled skills and SKILL.md presence.`);

  await verifySkillHashes(extensionSkills);

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
