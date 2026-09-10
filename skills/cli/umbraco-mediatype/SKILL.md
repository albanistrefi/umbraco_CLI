---
name: umbraco-mediatype
description: "Media type operations"
metadata:
  version: 0.4.15
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# mediatype

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco mediatype <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `mediatype children <id>` | Get child media types of a folder (paginated; --skip/--take/--all) |
| `mediatype export <id>` | Export a media type as a .udt document |
| `mediatype get <id-or-alias>` | Get media type by ID (or by exact alias) |
| `mediatype list` | List media types (paginated; --skip/--take/--all) |
| `mediatype search` | Search media types |

### children

```bash
umbraco mediatype children <id>
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

### export

```bash
umbraco mediatype export <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### get

```bash
umbraco mediatype get <id-or-alias>
```

Fetches a media type by GUID. A non-GUID argument is treated as an alias and resolved through the item search (exact, case-insensitive match); when nothing matches the command says so instead of issuing a request that can only 404.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### list

```bash
umbraco mediatype list
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--exclude-folders` | bool | false | Alias for --types-only |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--recursive` | bool | false | Walk media type folders recursively |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |
| `--types-only` | bool | false | Return media types only, excluding folders |

### search

```bash
umbraco mediatype search
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--params` | string | — | Search parameters as JSON; convenience flags fill in missing keys, --params wins on collisions |
| `--query` | string | — | Search query |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `mediatype create` | Create a media type |
| `mediatype delete <id>` | Delete a media type |
| `mediatype update <id>` | Update a media type (--json replaces, --merge-json merges) |

### create

```bash
umbraco mediatype create
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Create payload as JSON |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco mediatype create [flags] --dry-run

# 2. Execute with the same flags
umbraco mediatype create [flags]
```

### delete

```bash
umbraco mediatype delete <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco mediatype delete <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco mediatype delete <id> --force [flags]
```

### update

```bash
umbraco mediatype update <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--backup` | string | — | Save the current item to a JSON file before writing; bare --backup writes ./<collection>-<id>-<timestamp>.backup.json, --backup=<path> chooses the file. Undo with '<collection> restore-backup <file>' where available |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current resource before update (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco mediatype update <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco mediatype update <id> [flags]
```

## Discovering Commands

```bash
# Browse subcommands
umbraco mediatype --help

# Inspect a specific endpoint schema
umbraco schema mediatype.<method>
```
