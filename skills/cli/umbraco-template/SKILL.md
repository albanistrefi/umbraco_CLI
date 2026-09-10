---
name: umbraco-template
description: "Template operations"
metadata:
  version: 0.4.15
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# template

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco template <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `template get <id>` | Get template by ID |
| `template root` | Get root templates (paginated; --skip/--take/--all) |
| `template search` | Search templates |

### get

```bash
umbraco template get <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### root

```bash
umbraco template root
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

### search

```bash
umbraco template search
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
| `template create` | Create template |
| `template update <id>` | Update template |

### create

```bash
umbraco template create
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Create payload as JSON |
| `--print-template` | bool | false | Print an annotated JSON skeleton; substitute placeholders before passing to --json |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco template create [flags] --dry-run

# 2. Execute with the same flags
umbraco template create [flags]
```

### update

```bash
umbraco template update <id>
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
umbraco template update <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco template update <id> [flags]
```

## Discovering Commands

```bash
# Browse subcommands
umbraco template --help

# Inspect a specific endpoint schema
umbraco schema template.<method>
```
