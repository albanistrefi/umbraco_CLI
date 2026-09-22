---
name: umbraco-stylesheet
description: "Stylesheet operations"
metadata:
  version: 0.4.23
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# stylesheet

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco stylesheet <command> [flags]
```

## Overview

```text
Stylesheet operations. Stylesheets are keyed by their path under /wwwroot/css, not by a GUID.

Task → command:
  See the folders and files at the root            stylesheet list
  See what is inside a folder                      stylesheet children /theme
  Read a file's content                            stylesheet get /theme/site.css
  Save a file to disk verbatim                     stylesheet get /theme/site.css --out ./site.css
  Create a folder                                  stylesheet create-folder --path / --name theme
  Create a file                                    stylesheet create --path /theme --name site.css --content-file ./site.css
  Overwrite a file's content                       stylesheet update /theme/site.css --content-file ./site.css
  Rename a file (keeping its folder)               stylesheet rename /theme/site.css --name main.css
  Delete a file or an empty folder                 stylesheet delete /theme/site.css --force; stylesheet delete-folder /theme --force
```

## Read Commands

| Command | Description |
|---------|-------------|
| `stylesheet children <path>` | List stylesheets and folders inside a folder (paginated; --skip/--take/--all) |
| `stylesheet get <path>` | Get a stylesheet by path, including its content |
| `stylesheet list` | List stylesheets and folders at the root (paginated; --skip/--take/--all) |

### children

```bash
umbraco stylesheet children <path>
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
umbraco stylesheet get <path>
```

Fetches a stylesheet by its path (for example /Blog/Header.css). --out writes the content to disk verbatim, so a file can be round-tripped without shell quoting.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--out` | string | — | Write the content to this file verbatim and print a summary instead of the body |

### list

```bash
umbraco stylesheet list
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
| `stylesheet create --path <folder> --name <file>` | Create a stylesheet |
| `stylesheet create-folder --path <parent> --name <name>` | Create a stylesheet folder |
| `stylesheet delete <path>` | Permanently delete a stylesheet |
| `stylesheet delete-folder <path>` | Permanently delete an empty stylesheet folder |
| `stylesheet rename <path> --name <new-name>` | Rename a stylesheet, keeping it in its folder |
| `stylesheet update <path>` | Replace a stylesheet's content |

### create

```bash
umbraco stylesheet create --path <folder> --name <file>
```

Creates a stylesheet named --name inside the folder --path (use / for the root). The content comes from --content or, for anything with quotes or newlines, --content-file.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--content` | string | — | File content as text |
| `--content-file` | string | — | Read the file content verbatim from this local file |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--name` | string | — | File name, including the .css extension |
| `--path` | string | / | Folder to create the file in (/ for the root) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco stylesheet create --path <folder> --name <file> [flags] --dry-run

# 2. Execute with the same flags
umbraco stylesheet create --path <folder> --name <file> [flags]
```

### create-folder

```bash
umbraco stylesheet create-folder --path <parent> --name <name>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--name` | string | — | Folder name |
| `--path` | string | / | Parent folder (/ for the root) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco stylesheet create-folder --path <parent> --name <name> [flags] --dry-run

# 2. Execute with the same flags
umbraco stylesheet create-folder --path <parent> --name <name> [flags]
```

### delete

```bash
umbraco stylesheet delete <path>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco stylesheet delete <path> [flags] --dry-run

# 2. Execute with the same flags
umbraco stylesheet delete <path> --force [flags]
```

### delete-folder

```bash
umbraco stylesheet delete-folder <path>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco stylesheet delete-folder <path> [flags] --dry-run

# 2. Execute with the same flags
umbraco stylesheet delete-folder <path> --force [flags]
```

### rename

```bash
umbraco stylesheet rename <path> --name <new-name>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--name` | string | — | New file name, including the extension |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco stylesheet rename <path> --name <new-name> [flags] --dry-run

# 2. Execute with the same flags
umbraco stylesheet rename <path> --name <new-name> [flags]
```

### update

```bash
umbraco stylesheet update <path>
```

Replaces the whole content of a stylesheet. The Management API update model carries content only — there is no partial update, so pass the full file (--content-file round-trips `get --out`).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--content` | string | — | File content as text |
| `--content-file` | string | — | Read the file content verbatim from this local file |
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco stylesheet update <path> [flags] --dry-run

# 2. Execute with the same flags
umbraco stylesheet update <path> [flags]
```

## Discovering Commands

```bash
# Browse subcommands
umbraco stylesheet --help

# Inspect a specific endpoint schema
umbraco schema stylesheet.<method>
```
