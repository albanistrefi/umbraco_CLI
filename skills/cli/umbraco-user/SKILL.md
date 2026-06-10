---
name: umbraco-user
description: "Backoffice user management (accounts, state, groups, API credentials)"
metadata:
  version: 0.3.17
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

## Read Commands

| Command | Description |
|---------|-------------|
| `user client-credentials` | OAuth client credentials for API users (what this CLI logs in with) |
| `user current` | Get the user the CLI is authenticated as |
| `user get <id>` | Get a backoffice user by ID |
| `user list` | List backoffice users (paginated; --skip/--take/--all, --filter for substring search) |
| `user permissions --ids <id,...>` | Check the current user's permissions on specific items |

### client-credentials

```bash
umbraco user client-credentials
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
umbraco user get <id>
```

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
| `user create` | Create a backoffice user |
| `user delete <id>` | Permanently delete a backoffice user |
| `user disable --ids <id,...>` | Disable user accounts (they keep existing but cannot log in) |
| `user enable --ids <id,...>` | Enable disabled user accounts |
| `user invite` | Invite a user by email (they choose their own password) |
| `user set-groups` | Replace the group memberships of one or more users |
| `user unlock --ids <id,...>` | Unlock user accounts locked out by failed logins |
| `user update <id>` | Update a backoffice user |

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
# 1. Dry run first
umbraco user create --dry-run

# 2. Execute
umbraco user create
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
# 1. Dry run first
umbraco user delete <id> --dry-run

# 2. Execute
umbraco user delete <id>
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
# 1. Dry run first
umbraco user disable --ids <id,...> --dry-run

# 2. Execute
umbraco user disable --ids <id,...>
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
# 1. Dry run first
umbraco user enable --ids <id,...> --dry-run

# 2. Execute
umbraco user enable --ids <id,...>
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
# 1. Dry run first
umbraco user invite --dry-run

# 2. Execute
umbraco user invite
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
# 1. Dry run first
umbraco user set-groups --dry-run

# 2. Execute
umbraco user set-groups
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
# 1. Dry run first
umbraco user unlock --ids <id,...> --dry-run

# 2. Execute
umbraco user unlock --ids <id,...>
```

### update

```bash
umbraco user update <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current resource before update (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco user update <id> --dry-run

# 2. Execute
umbraco user update <id>
```

## Discovering Commands

```bash
# Browse subcommands
umbraco user --help

# Inspect a specific endpoint schema
umbraco schema user.<method>
```
