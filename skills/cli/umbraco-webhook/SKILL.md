---
name: umbraco-webhook
description: "Webhook management (the Management API's outbound event notifications)"
metadata:
  version: 0.4.0
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# webhook

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco webhook <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `webhook events` | List the event aliases webhooks can subscribe to |
| `webhook get <id>` | Get a webhook by ID |
| `webhook list` | List webhooks (paginated; --skip/--take/--all) |
| `webhook logs [webhook-id]` | List webhook delivery logs, optionally scoped to one webhook |

### events

```bash
umbraco webhook events
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

### get

```bash
umbraco webhook get <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### list

```bash
umbraco webhook list
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

### logs

```bash
umbraco webhook logs [webhook-id]
```

GET /webhook/logs, or /webhook/{id}/logs when a webhook ID is given. Each entry carries the event alias, target URL, response status, and retry count — the audit trail for 'did my integration fire'.

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
| `webhook create` | Create a webhook |
| `webhook delete <id>` | Permanently delete a webhook |
| `webhook update <id>` | Update a webhook |

### create

```bash
umbraco webhook create
```

POST /webhook. Required fields: url, events (aliases from 'webhook events'), enabled, contentTypeKeys (empty array = all content types), headers (empty object = none). Use --print-template for the payload shape.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Create payload as JSON |
| `--print-template` | bool | false | Print an annotated JSON skeleton; substitute placeholders before passing to --json |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco webhook create --dry-run

# 2. Execute
umbraco webhook create
```

### delete

```bash
umbraco webhook delete <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco webhook delete <id> --dry-run

# 2. Execute
umbraco webhook delete <id>
```

### update

```bash
umbraco webhook update <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current resource before update (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco webhook update <id> --dry-run

# 2. Execute
umbraco webhook update <id>
```

## Discovering Commands

```bash
# Browse subcommands
umbraco webhook --help

# Inspect a specific endpoint schema
umbraco schema webhook.<method>
```
