---
name: umbraco-partial-view
description: "Partial view operations"
metadata:
  version: 0.4.23
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# partial-view

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco partial-view <command> [flags]
```

## Overview

```text
Partial view operations. Partial views are keyed by their path under /Views/Partials, not by a GUID.

Task → command:
  See the folders and files at the root            partial-view list
  See what is inside a folder                      partial-view children /Blog
  Read a file's content                            partial-view get /Blog/Header.cshtml
  Save a file to disk verbatim                     partial-view get /Blog/Header.cshtml --out ./Header.cshtml
  Create a folder                                  partial-view create-folder --path / --name Blog
  Create a file                                    partial-view create --path /Blog --name Header.cshtml --content-file ./Header.cshtml
  Overwrite a file's content                       partial-view update /Blog/Header.cshtml --content-file ./Header.cshtml
  Rename a file (keeping its folder)               partial-view rename /Blog/Header.cshtml --name Top.cshtml
  Delete a file or an empty folder                 partial-view delete /Blog/Header.cshtml --force; partial-view delete-folder /Blog --force
  Start from a built-in snippet                    partial-view snippets; partial-view snippet Breadcrumb
```

## Read Commands

| Command | Description |
|---------|-------------|
| `partial-view children <path>` | List partial views and folders inside a folder (paginated; --skip/--take/--all) |
| `partial-view get <path>` | Get a partial view by path, including its content |
| `partial-view list` | List partial views and folders at the root (paginated; --skip/--take/--all) |
| `partial-view snippet <id>` | Get a built-in partial view snippet, including its content |
| `partial-view snippets` | List the built-in partial view snippets (paginated; --skip/--take/--all) |

### children

```bash
umbraco partial-view children <path>
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
umbraco partial-view get <path>
```

Fetches a partial view by its path (for example /Blog/Header.cshtml). --out writes the content to disk verbatim, so a file can be round-tripped without shell quoting.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--out` | string | — | Write the content to this file verbatim and print a summary instead of the body |

### list

```bash
umbraco partial-view list
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

### snippet

```bash
umbraco partial-view snippet <id>
```

Fetches one snippet by its id (as listed by `partial-view snippets`). Its content is the starting point Umbraco offers in the backoffice when creating a partial view.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### snippets

```bash
umbraco partial-view snippets
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
| `partial-view create --path <folder> --name <file>` | Create a partial view |
| `partial-view create-folder --path <parent> --name <name>` | Create a partial view folder |
| `partial-view delete <path>` | Permanently delete a partial view |
| `partial-view delete-folder <path>` | Permanently delete an empty partial view folder |
| `partial-view rename <path> --name <new-name>` | Rename a partial view, keeping it in its folder |
| `partial-view update <path>` | Replace a partial view's content |

### create

```bash
umbraco partial-view create --path <folder> --name <file>
```

Creates a partial view named --name inside the folder --path (use / for the root). The content comes from --content or, for anything with quotes or newlines, --content-file.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--content` | string | — | File content as text |
| `--content-file` | string | — | Read the file content verbatim from this local file |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--name` | string | — | File name, including the .cshtml extension |
| `--path` | string | / | Folder to create the file in (/ for the root) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco partial-view create --path <folder> --name <file> [flags] --dry-run

# 2. Execute with the same flags
umbraco partial-view create --path <folder> --name <file> [flags]
```

### create-folder

```bash
umbraco partial-view create-folder --path <parent> --name <name>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--name` | string | — | Folder name |
| `--path` | string | / | Parent folder (/ for the root) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco partial-view create-folder --path <parent> --name <name> [flags] --dry-run

# 2. Execute with the same flags
umbraco partial-view create-folder --path <parent> --name <name> [flags]
```

### delete

```bash
umbraco partial-view delete <path>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco partial-view delete <path> [flags] --dry-run

# 2. Execute with the same flags
umbraco partial-view delete <path> --force [flags]
```

### delete-folder

```bash
umbraco partial-view delete-folder <path>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco partial-view delete-folder <path> [flags] --dry-run

# 2. Execute with the same flags
umbraco partial-view delete-folder <path> --force [flags]
```

### rename

```bash
umbraco partial-view rename <path> --name <new-name>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--name` | string | — | New file name, including the extension |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco partial-view rename <path> --name <new-name> [flags] --dry-run

# 2. Execute with the same flags
umbraco partial-view rename <path> --name <new-name> [flags]
```

### update

```bash
umbraco partial-view update <path>
```

Replaces the whole content of a partial view. The Management API update model carries content only — there is no partial update, so pass the full file (--content-file round-trips `get --out`).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--content` | string | — | File content as text |
| `--content-file` | string | — | Read the file content verbatim from this local file |
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco partial-view update <path> [flags] --dry-run

# 2. Execute with the same flags
umbraco partial-view update <path> [flags]
```

## Discovering Commands

```bash
# Browse subcommands
umbraco partial-view --help

# Inspect a specific endpoint schema
umbraco schema partial-view.<method>
```
