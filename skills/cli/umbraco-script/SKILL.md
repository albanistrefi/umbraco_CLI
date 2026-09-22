---
name: umbraco-script
description: "Script operations"
metadata:
  version: 0.4.23
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# script

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco script <command> [flags]
```

## Overview

```text
Script operations. Scripts are keyed by their path under /wwwroot/scripts, not by a GUID.

Task → command:
  See the folders and files at the root            script list
  See what is inside a folder                      script children /vendor
  Read a file's content                            script get /vendor/app.js
  Save a file to disk verbatim                     script get /vendor/app.js --out ./app.js
  Create a folder                                  script create-folder --path / --name vendor
  Create a file                                    script create --path /vendor --name app.js --content-file ./app.js
  Overwrite a file's content                       script update /vendor/app.js --content-file ./app.js
  Rename a file (keeping its folder)               script rename /vendor/app.js --name main.js
  Delete a file or an empty folder                 script delete /vendor/app.js --force; script delete-folder /vendor --force
```

## Read Commands

| Command | Description |
|---------|-------------|
| `script children <path>` | List scripts and folders inside a folder (paginated; --skip/--take/--all) |
| `script get <path>` | Get a script by path, including its content |
| `script list` | List scripts and folders at the root (paginated; --skip/--take/--all) |

### children

```bash
umbraco script children <path>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

### get

```bash
umbraco script get <path>
```

Fetches a script by its path (for example /Blog/Header.js). --out writes the content to disk verbatim, so a file can be round-tripped without shell quoting.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--out` | string | — | Write the content to this file verbatim and print a summary instead of the body |

### list

```bash
umbraco script list
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `script create --path <folder> --name <file>` | Create a script |
| `script create-folder --path <parent> --name <name>` | Create a script folder |
| `script delete <path>` | Permanently delete a script |
| `script delete-folder <path>` | Permanently delete an empty script folder |
| `script rename <path> --name <new-name>` | Rename a script, keeping it in its folder |
| `script update <path>` | Replace a script's content |

### create

```bash
umbraco script create --path <folder> --name <file>
```

Creates a script named --name inside the folder --path (use / for the root). The content comes from --content or, for anything with quotes or newlines, --content-file.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--content` | string | — | File content as text |
| `--content-file` | string | — | Read the file content verbatim from this local file |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--name` | string | — | File name, including the .js extension |
| `--path` | string | / | Folder to create the file in (/ for the root) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco script create --path <folder> --name <file> [flags] --dry-run

# 2. Execute with the same flags
umbraco script create --path <folder> --name <file> [flags]
```

### create-folder

```bash
umbraco script create-folder --path <parent> --name <name>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--name` | string | — | Folder name |
| `--path` | string | / | Parent folder (/ for the root) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco script create-folder --path <parent> --name <name> [flags] --dry-run

# 2. Execute with the same flags
umbraco script create-folder --path <parent> --name <name> [flags]
```

### delete

```bash
umbraco script delete <path>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco script delete <path> [flags] --dry-run

# 2. Execute with the same flags
umbraco script delete <path> --force [flags]
```

### delete-folder

```bash
umbraco script delete-folder <path>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco script delete-folder <path> [flags] --dry-run

# 2. Execute with the same flags
umbraco script delete-folder <path> --force [flags]
```

### rename

```bash
umbraco script rename <path> --name <new-name>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--name` | string | — | New file name, including the extension |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco script rename <path> --name <new-name> [flags] --dry-run

# 2. Execute with the same flags
umbraco script rename <path> --name <new-name> [flags]
```

### update

```bash
umbraco script update <path>
```

Replaces the whole content of a script. The Management API update model carries content only — there is no partial update, so pass the full file (--content-file round-trips `get --out`).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--content` | string | — | File content as text |
| `--content-file` | string | — | Read the file content verbatim from this local file |
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco script update <path> [flags] --dry-run

# 2. Execute with the same flags
umbraco script update <path> [flags]
```

## Discovering Commands

```bash
# Browse subcommands
umbraco script --help

# Inspect a specific endpoint schema
umbraco schema script.<method>
```
