---
name: umbraco-user
description: "Backoffice user management (accounts, state, groups, API credentials)"
metadata:
  version: 0.4.14
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# user

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco user <command> [flags]
```

## Overview

```text
Manages the backoffice users the CLI itself authenticates as — not front-office members (see 'member'). 'user client-credentials' manages the OAuth client IDs/secrets that API users like this CLI log in with.
```

## Read Commands

| Command | Description |
|---------|-------------|
| `user client-credentials list <user-id>` | List the client IDs registered for an API user |
| `user current` | Get the user the CLI is authenticated as |
| `user get <id> [<id>...]` | Get backoffice users by ID |
| `user list` | List backoffice users (paginated; --skip/--take/--all, --filter for substring search) |
| `user permissions --ids <id,...>` | Check the current user's permissions on specific items |

### client-credentials list

```bash
umbraco user client-credentials list <user-id>
```

### current

```bash
umbraco user current
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### get

```bash
umbraco user get <id> [<id>...]
```

GET /user/{id} for a single ID; several IDs fetch in one round trip via GET /user/batch (Umbraco 18.1+).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### list

```bash
umbraco user list
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--filter` | string | — | Substring filter against user name/email |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

### permissions

```bash
umbraco user permissions --ids <id,...>
```

GET /user/current/permissions[/document|/media]. Lets an agent verify it may write or publish a node before issuing the mutation. --type selects the permission surface: entity (default), document, or media.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ids` | string | — | Comma-separated entity GUIDs to check (required) |
| `--type` | string | entity | Permission surface: entity, document, or media |

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `user client-credentials create <user-id>` | Register a client ID/secret pair on an API user |
| `user client-credentials delete <user-id> <client-id>` | Remove a client ID from an API user (revokes its access) |
| `user create` | Create a backoffice user |
| `user delete <id>` | Permanently delete a backoffice user |
| `user disable --ids <id,...>` | Disable user accounts (they keep existing but cannot log in) |
| `user enable --ids <id,...>` | Enable disabled user accounts |
| `user invite` | Invite a user by email (they choose their own password) |
| `user set-groups` | Replace the group memberships of one or more users |
| `user set-language <iso-code>` | Set the current user's backoffice UI language |
| `user unlock --ids <id,...>` | Unlock user accounts locked out by failed logins |
| `user update <id>` | Update a backoffice user |

### client-credentials create

```bash
umbraco user client-credentials create <user-id>
```

POST /user/{id}/client-credentials. The user must be of kind Api ('user create' with "kind":"Api"). Client IDs are conventionally prefixed umbraco-back-office-.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--client-id` | string | — | OAuth client ID (required) |
| `--client-secret` | string | — | OAuth client secret (required) |
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco user client-credentials create <user-id> [flags] --dry-run

# 2. Execute with the same flags
umbraco user client-credentials create <user-id> [flags]
```

### client-credentials delete

```bash
umbraco user client-credentials delete <user-id> <client-id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm revoking the credential |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco user client-credentials delete <user-id> <client-id> [flags] --dry-run

# 2. Execute with the same flags
umbraco user client-credentials delete <user-id> <client-id> --force [flags]
```

### create

```bash
umbraco user create
```

POST /user. Required: email, userName, name, userGroupIds ([{"id":"<guid>"}] from 'user-group list'), kind ("Default" for humans, "Api" for credential-only API users). API-kind users get credentials via 'user client-credentials create'.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Create payload as JSON |
| `--print-template` | bool | false | Print an annotated JSON skeleton; substitute placeholders before passing to --json |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco user create [flags] --dry-run

# 2. Execute with the same flags
umbraco user create [flags]
```

### delete

```bash
umbraco user delete <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco user delete <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco user delete <id> --force [flags]
```

### disable

```bash
umbraco user disable --ids <id,...>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--ids` | string | — | Comma-separated user GUIDs (required) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco user disable --ids <id,...> [flags] --dry-run

# 2. Execute with the same flags
umbraco user disable --ids <id,...> [flags]
```

### enable

```bash
umbraco user enable --ids <id,...>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--ids` | string | — | Comma-separated user GUIDs (required) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco user enable --ids <id,...> [flags] --dry-run

# 2. Execute with the same flags
umbraco user enable --ids <id,...> [flags]
```

### invite

```bash
umbraco user invite
```

POST /user/invite. Same required fields as 'user create' minus kind, plus an optional message included in the invitation email. Requires the server to have SMTP configured.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Invite payload as JSON |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco user invite [flags] --dry-run

# 2. Execute with the same flags
umbraco user invite [flags]
```

### set-groups

```bash
umbraco user set-groups
```

POST /user/set-user-groups. Replaces each listed user's groups with exactly the listed group set. Group GUIDs come from 'user-group list'.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--group-ids` | string | — | Comma-separated user-group GUIDs; the users' groups become exactly this set |
| `--user-ids` | string | — | Comma-separated user GUIDs (required) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco user set-groups [flags] --dry-run

# 2. Execute with the same flags
umbraco user set-groups [flags]
```

### set-language

```bash
umbraco user set-language <iso-code>
```

PUT /user/current/profile (Umbraco 18.1+). Sets the backoffice UI language of the account the CLI authenticates as (e.g. en-US, da-DK).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco user set-language <iso-code> [flags] --dry-run

# 2. Execute with the same flags
umbraco user set-language <iso-code> [flags]
```

### unlock

```bash
umbraco user unlock --ids <id,...>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--ids` | string | — | Comma-separated user GUIDs (required) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco user unlock --ids <id,...> [flags] --dry-run

# 2. Execute with the same flags
umbraco user unlock --ids <id,...> [flags]
```

### update

```bash
umbraco user update <id>
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
umbraco user update <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco user update <id> [flags]
```

## Discovering Commands

```bash
# Browse subcommands
umbraco user --help

# Inspect a specific endpoint schema
umbraco schema user.<method>
```
