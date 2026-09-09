---
name: umbraco-api
description: "Call an authenticated raw Umbraco Management API endpoint"
metadata:
  version: 0.4.14
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

--raw-path sends the path relative to the host root instead (any endpoint on the same host, e.g. /umbraco/automate/management/api/v1/automations or /media/abc/logo.svg).
--form field=value / field=@path sends multipart/form-data instead of JSON (e.g. POST /temporary-file with --form id=<uuid> --form file=@./logo.svg).
--header 'Key: Value' adds or overrides request headers. Every request already carries User-Agent umbraco-cli/<version>.

Response bodies are decoded as JSON (or text) for the structured output; binary responses are not preserved that way — use --out <file> to save a GET response verbatim.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--body` | string | — | JSON request body, or @path to read JSON from a file |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--form` | stringArray | [] | Multipart form field as field=value or field=@path for a file (repeatable; replaces the JSON body) |
| `--header` | stringArray | [] | Extra request header as 'Key: Value' (repeatable) |
| `--out` | string | — | GET only: write the response body verbatim to this file (binary-safe; use for /media/... assets with --raw-path) and print a summary instead of the body |
| `--raw-path` | bool | false | Send the path relative to the host root instead of /umbraco/management/api/v1 |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco api <method> <path> [flags] --dry-run

# 2. Execute with the same flags
umbraco api <method> <path> [flags]
```

## Discovering Commands

```bash
umbraco api --help
```
