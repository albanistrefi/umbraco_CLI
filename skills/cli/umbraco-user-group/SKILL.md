---
name: umbraco-user-group
description: "Backoffice user group management (permission sets)"
metadata:
  version: 0.4.5
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# user-group

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco user-group <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `user-group get <id>` | Get a user group by ID |
| `user-group list` | List user groups (paginated; --skip/--take/--all, --filter for substring search) |

### get

```bash
umbraco user-group get <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### list

```bash
umbraco user-group list
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--filter` | string | — | Substring filter against group names |
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
| `user-group add-users <group-id> --ids <id,...>` | Add users to a user group |
| `user-group create` | Create a user group |
| `user-group delete <id>` | Permanently delete a user group |
| `user-group remove-users <group-id> --ids <id,...>` | Remove users from a user group |
| `user-group update <id>` | Update a user group |

### add-users

```bash
umbraco user-group add-users <group-id> --ids <id,...>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--ids` | string | — | Comma-separated user GUIDs (required) |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco user-group add-users <group-id> --ids <id,...> --dry-run

# 2. Execute
umbraco user-group add-users <group-id> --ids <id,...>
```

### create

```bash
umbraco user-group create
```

POST /user-group. The model is permission-heavy; start from --print-template or 'user-group get' an existing group. sections use the umb-prefixed aliases (Umb.Section.Content, ...); permissions use single-letter verb codes matching the backoffice.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Create payload as JSON |
| `--print-template` | bool | false | Print an annotated JSON skeleton; substitute placeholders before passing to --json |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco user-group create --dry-run

# 2. Execute
umbraco user-group create
```

### delete

```bash
umbraco user-group delete <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco user-group delete <id> --dry-run

# 2. Execute
umbraco user-group delete <id>
```

### remove-users

```bash
umbraco user-group remove-users <group-id> --ids <id,...>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--ids` | string | — | Comma-separated user GUIDs (required) |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco user-group remove-users <group-id> --ids <id,...> --dry-run

# 2. Execute
umbraco user-group remove-users <group-id> --ids <id,...>
```

### update

```bash
umbraco user-group update <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current resource before update (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco user-group update <id> --dry-run

# 2. Execute
umbraco user-group update <id>
```

## Discovering Commands

```bash
# Browse subcommands
umbraco user-group --help

# Inspect a specific endpoint schema
umbraco schema user-group.<method>
```
