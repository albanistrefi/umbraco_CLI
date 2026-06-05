---
name: umbraco-member
description: "Front-office member operations (login, profile, groups)"
metadata:
  version: 0.3.15
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# member

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco member <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `member get <id>` | Get a member by ID |
| `member list` | List members (paginated; use --filter for substring search) |
| `member search <query>` | Search members by username/email substring (shorthand for 'member list --filter') |

### get

```bash
umbraco member get <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |

### list

```bash
umbraco member list
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (defaults to all; pass id,username,email,... for agent-friendly summaries) |
| `--filter` | string | — | Substring filter against member username/email |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--skip` | int | 0 | Skip count |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | 0 | Take count (0 = server default) |

### search

```bash
umbraco member search <query>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | id,username,email,isApproved,isLockedOut,isTwoFactorEnabled,failedPasswordAttempts,groups | Limit response fields (default surfaces login-diagnosis fields; pass empty string for full payload) |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | 0 | Maximum results |

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `member create` | Create a member |
| `member delete <id>` | Delete a member |
| `member set-groups <id>` | Replace or modify a member's group memberships |
| `member update <id>` | Update a member (fetch-and-merge) |
| `member update-properties <id>` | Update member custom property values (merges into values[] by alias) |

### create

```bash
umbraco member create
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Create payload as JSON |
| `--print-template` | bool | false | Print an annotated JSON skeleton; substitute placeholders before passing to --json |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco member create --dry-run

# 2. Execute
umbraco member create
```

### delete

```bash
umbraco member delete <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco member delete <id> --dry-run

# 2. Execute
umbraco member delete <id>
```

### set-groups

```bash
umbraco member set-groups <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--add-groups` | string | — | Comma-separated group GUIDs to add (idempotent) |
| `--dry-run` | bool | false | Validate request without executing |
| `--groups` | string | — | Comma-separated group GUIDs; replaces the member's groups[] with this exact set |
| `--remove-groups` | string | — | Comma-separated group GUIDs to remove (idempotent) |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco member set-groups <id> --dry-run

# 2. Execute
umbraco member set-groups <id>
```

### update

```bash
umbraco member update <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Update payload as JSON; merged into the current member so fields not mentioned are preserved |
| `--merge-json` | string | — | Alias for --json |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco member update <id> --dry-run

# 2. Execute
umbraco member update <id>
```

### update-properties

```bash
umbraco member update-properties <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Validate request without executing |
| `--json` | string | — | Properties payload as JSON (object / array / envelope) |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco member update-properties <id> --dry-run

# 2. Execute
umbraco member update-properties <id>
```

## Discovering Commands

```bash
# Browse subcommands
umbraco member --help

# Inspect a specific endpoint schema
umbraco schema member.<method>
```
