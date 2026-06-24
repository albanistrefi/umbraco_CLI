---
name: umbraco-api
description: "Call an authenticated raw Umbraco Management API endpoint"
metadata:
  version: 0.4.4
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# api

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco api <method> <path> [flags]
```

## Command

### api

```bash
umbraco api <method> <path>
```

Call a core Umbraco Management API endpoint that does not have a curated CLI command yet.

Pass paths relative to /umbraco/management/api/v1, for example /item/document/ancestors?id=a&id=b.
Full Management API paths are also accepted and normalized to the core API root.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--body` | string | — | JSON request body, or @path to read JSON from a file |
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco api <method> <path> --dry-run

# 2. Execute
umbraco api <method> <path>
```

## Discovering Commands

```bash
umbraco api --help
```
