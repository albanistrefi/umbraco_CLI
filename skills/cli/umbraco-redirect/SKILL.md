---
name: umbraco-redirect
description: "Redirect URL management (tracked 301s from renamed/moved documents)"
metadata:
  version: 0.4.16
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# redirect

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco redirect <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `redirect get <document-id>` | List redirects recorded for one document |
| `redirect list` | List tracked redirects (paginated; use --filter for URL substring search) |
| `redirect status` | Get redirect tracking status (enabled/disabled) |

### get

```bash
umbraco redirect get <document-id>
```

GET /redirect-management/{id}. Returns the redirects pointing at the given document key, paginated.

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

### list

```bash
umbraco redirect list
```

GET /redirect-management. Lists the redirects Umbraco recorded when published documents were renamed or moved. --filter matches against the original URL.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--filter` | string | — | Substring filter against the original redirect URL |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

### status

```bash
umbraco redirect status
```

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `redirect delete <id>` | Delete a tracked redirect |
| `redirect disable` | Disable redirect URL tracking |
| `redirect enable` | Enable redirect URL tracking |

### delete

```bash
umbraco redirect delete <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco redirect delete <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco redirect delete <id> --force [flags]
```

### disable

```bash
umbraco redirect disable
```

POST /redirect-management/status?status=Disabled. Toggles redirect URL tracking site-wide; the inverse command reverses it.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco redirect disable [flags] --dry-run

# 2. Execute with the same flags
umbraco redirect disable [flags]
```

### enable

```bash
umbraco redirect enable
```

POST /redirect-management/status?status=Enabled. Toggles redirect URL tracking site-wide; the inverse command reverses it.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco redirect enable [flags] --dry-run

# 2. Execute with the same flags
umbraco redirect enable [flags]
```

## Discovering Commands

```bash
# Browse subcommands
umbraco redirect --help

# Inspect a specific endpoint schema
umbraco schema redirect.<method>
```
