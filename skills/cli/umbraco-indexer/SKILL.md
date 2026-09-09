---
name: umbraco-indexer
description: "Examine search index operations"
metadata:
  version: 0.4.14
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# indexer

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco indexer <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `indexer get <index-name>` | Get one Examine index (health, document count, fields) |
| `indexer list` | List Examine indexes with health and document counts |

### get

```bash
umbraco indexer get <index-name>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### list

```bash
umbraco indexer list
```

GET /indexer. The classic first stop when search results are missing or stale: healthStatus.status of Rebuilding, Unhealthy, or Corrupt explains it.

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
| `indexer rebuild <index-name>` | Rebuild an Examine index |

### rebuild

```bash
umbraco indexer rebuild <index-name>
```

POST /indexer/{indexName}/rebuild. Rebuilds the index from scratch — the standard fix for missing or stale search results. Expensive on large indexes; with --wait, polls the index until healthStatus leaves Rebuilding or --timeout elapses.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm the rebuild |
| `--poll-interval` | duration | 1s | How often to poll when --wait is set |
| `--timeout` | duration | 1m0s | How long to wait when --wait is set (e.g. 30s, 2m) |
| `--wait` | bool | false | Poll the index after triggering the rebuild until healthStatus leaves Rebuilding or --timeout elapses |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco indexer rebuild <index-name> [flags] --dry-run

# 2. Execute with the same flags
umbraco indexer rebuild <index-name> --force [flags]
```

## Discovering Commands

```bash
# Browse subcommands
umbraco indexer --help

# Inspect a specific endpoint schema
umbraco schema indexer.<method>
```
