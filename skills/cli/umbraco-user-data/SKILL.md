---
name: umbraco-user-data
description: "Key/value data stored for the authenticated user"
metadata:
  version: 0.4.21
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# user-data

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco user-data <command> [flags]
```

## Overview

```text
Key/value data stored for the authenticated user.

Every endpoint operates on the account the CLI is authenticated as; there is
no way to read or write another user's data.

Task → command:
  What is stored for me?                       user-data list --all
  Narrow to one group or identifier            user-data list --groups <g> --identifiers <i>
  Read one entry by key                        user-data get <key>
  Store a new entry                            user-data create --group <g> --identifier <i> --value <v>
  Replace an entry (all fields required)       user-data update <key> --group <g> --identifier <i> --value <v>
```

## Read Commands

| Command | Description |
|---------|-------------|
| `user-data get <key>` | Get one user data entry by key (GUID) |
| `user-data list` | List the authenticated user's data entries (paginated; --skip/--take/--all) |

### get

```bash
umbraco user-data get <key>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### list

```bash
umbraco user-data list
```

GET /user-data. --groups and --identifiers are comma-separated lists sent as repeated query values. --params wins on key collisions.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--groups` | string | — | Comma-separated groups to filter by (repeated ?groups=) |
| `--identifiers` | string | — | Comma-separated identifiers to filter by (repeated ?identifiers=) |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `user-data create` | Create a user data entry |
| `user-data update <key>` | Replace a user data entry (all fields required) |

### create

```bash
umbraco user-data create
```

POST /user-data. Either pass the full payload via --json, or use the convenience flags (--group, --identifier and --value required). --key is optional; the server assigns one when it is omitted.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--group` | string | — | Entry group |
| `--identifier` | string | — | Entry identifier within the group |
| `--json` | string | — | Create payload as JSON |
| `--key` | string | — | Entry key (GUID); the server assigns one when omitted |
| `--value` | string | — | Entry value |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco user-data create [flags] --dry-run

# 2. Execute with the same flags
umbraco user-data create [flags]
```

### update

```bash
umbraco user-data update <key>
```

PUT /user-data with the key in the body. Either pass the full payload via --json, or use the convenience flags (--group, --identifier and --value are all required; the update model has no partial form).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--group` | string | — | Entry group |
| `--identifier` | string | — | Entry identifier within the group |
| `--json` | string | — | Full replacement payload as JSON |
| `--value` | string | — | Entry value |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco user-data update <key> [flags] --dry-run

# 2. Execute with the same flags
umbraco user-data update <key> [flags]
```

## Discovering Commands

```bash
# Browse subcommands
umbraco user-data --help

# Inspect a specific endpoint schema
umbraco schema user-data.<method>
```
