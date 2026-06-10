---
name: umbraco-language
description: "Language and culture management for variant content"
metadata:
  version: 0.3.17
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# language

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco language <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `language cultures` | List the ISO cultures available for new languages (paginated; --skip/--take/--all) |
| `language default` | Get the default language |
| `language get <iso-code>` | Get a language by ISO code (e.g. en-US) |
| `language list` | List configured languages (paginated; --skip/--take/--all) |

### cultures

```bash
umbraco language cultures
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

### default

```bash
umbraco language default
```

### get

```bash
umbraco language get <iso-code>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### list

```bash
umbraco language list
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
| `language create` | Create a language |
| `language delete <iso-code>` | Permanently delete a language (content variants for it become unreachable) |
| `language update <iso-code>` | Update a language |

### create

```bash
umbraco language create
```

POST /language. Either pass the full payload via --json, or use the convenience flags (--iso-code and --name required). Valid ISO codes come from 'language cultures'.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--default` | bool | false | Make this the default language |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--fallback` | string | — | Fallback language ISO code |
| `--iso-code` | string | — | Language ISO code (e.g. da-DK) |
| `--json` | string | — | Create payload as JSON |
| `--mandatory` | bool | false | Require this language before content can publish |
| `--name` | string | — | Display name (e.g. Danish) |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco language create --dry-run

# 2. Execute
umbraco language create
```

### delete

```bash
umbraco language delete <iso-code>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco language delete <iso-code> --dry-run

# 2. Execute
umbraco language delete <iso-code>
```

### update

```bash
umbraco language update <iso-code>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current resource before update (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco language update <iso-code> --dry-run

# 2. Execute
umbraco language update <iso-code>
```

## Discovering Commands

```bash
# Browse subcommands
umbraco language --help

# Inspect a specific endpoint schema
umbraco schema language.<method>
```
