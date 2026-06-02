---
name: umbraco-dictionary
description: "Dictionary item and translation key operations"
metadata:
  version: 0.3.13
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# dictionary

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco dictionary <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `dictionary export` | Export all dictionary items to JSON |
| `dictionary get [id]` | Get a dictionary item by ID or key |
| `dictionary list` | List dictionary items |

### export

```bash
umbraco dictionary export
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--file` | string | — | Write exported JSON to a file instead of stdout |

### get

```bash
umbraco dictionary get [id]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--key` | string | — | Dictionary key name |

### list

```bash
umbraco dictionary list
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |
| `--filter` | string | — | Filter dictionary items by key name |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--skip` | int | 0 | Pagination offset |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | 100 | Pagination page size |

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `dictionary create` | Create a dictionary item |
| `dictionary import` | Import dictionary items from JSON |

### create

```bash
umbraco dictionary create
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Full JSON payload |
| `--key` | string | — | Dictionary key name |
| `--parent-id` | string | — | Optional parent dictionary item ID |
| `--translation` | stringArray | [] | Translation in isoCode=value format; repeat for multiple locales |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco dictionary create --dry-run

# 2. Execute
umbraco dictionary create
```

### import

```bash
umbraco dictionary import
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--batch-size` | int | 5 | Maximum concurrent create or update requests (1-10) |
| `--dry-run` | bool | false | Plan the import without writing changes |
| `--file` | string | — | Path to the dictionary import JSON file |
| `--skip-existing` | bool | true | Skip items that already exist |
| `--update-existing` | bool | false | Merge translations into existing items |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco dictionary import --dry-run

# 2. Execute
umbraco dictionary import
```

## Discovering Commands

```bash
# Browse subcommands
umbraco dictionary --help

# Inspect a specific endpoint schema
umbraco schema dictionary.<method>
```
