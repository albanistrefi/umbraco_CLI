---
name: umbraco-logs
description: "Log and diagnostics operations"
metadata:
  version: 0.4.16
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# logs

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco logs <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `logs errors` | List Error/Fatal log entries, optionally grouped into distinct error classes |
| `logs level-count` | Get count per level |
| `logs list` | List log entries |
| `logs search` | Search logs |
| `logs tail` | Follow new log entries as they arrive (client-side polling) |
| `logs templates` | List paginated log message templates |

### errors

```bash
umbraco logs errors
```

GET /log-viewer/log filtered to Error and Fatal levels. --distinct groups entries into error classes by fingerprint (message template + normalized exception head, so two SQL violations with different constraint names are different classes) and reports count, first/last seen, and an example per class, sorted newest-first-seen so new breakage tops the list. --suppress drops known-chronic classes by fingerprint; --suppress-contains drops classes whose template or exception matches a substring. Suppressed classes are counted in the summary so they never vanish silently.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--distinct` | bool | false | Group entries into fingerprinted error classes |
| `--max-entries` | int | 10000 | Maximum entries to scan in the window |
| `--since` | string | — | Start of the window (ISO/RFC3339); default: 24 hours ago |
| `--suppress` | stringArray | [] | Fingerprint of a known-chronic error class to drop (repeatable) |
| `--suppress-contains` | stringArray | [] | Drop classes whose template or exception contains this substring (repeatable) |
| `--until` | string | — | End of the window (ISO/RFC3339); default: now |

### level-count

```bash
umbraco logs level-count
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--from` | string | — | Start date (ISO) |
| `--params` | string | — | Filter params as JSON |
| `--to` | string | — | End date (ISO) |

### list

```bash
umbraco logs list
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--around` | string | — | Center timestamp for a strict time window (ISO/RFC3339) |
| `--contains` | string | — | Client-side text contains filter across message, exception, and properties |
| `--correlation-id` | string | — | Client-side correlation/request ID contains filter |
| `--count-by` | string | — | Return counts grouped by level, source, or path |
| `--cursor` | string | — | Pagination cursor returned as nextCursor |
| `--filter-expression` | string | — | Serilog filter expression |
| `--flat` | bool | false | Return stable flat JSON entries with properties as an object |
| `--from` | string | — | Start date/time (ISO/RFC3339); enforced client-side |
| `--level` | string | — | Log level |
| `--minutes` | int | 5 | Minutes before and after --around |
| `--params` | string | — | Filter params as JSON (accepted keys: startDate,endDate,skip,take,filterExpression,logLevel) |
| `--path` | string | — | Client-side RequestPath contains filter |
| `--redact` | string | — | Comma-separated redaction modes: emails,secrets,tokens,all |
| `--redact-default` | bool | false | Redact emails, secrets, and tokens from output |
| `--skip` | int | -1 | Skip count |
| `--source-context` | string | — | Client-side SourceContext contains filter |
| `--take` | int | -1 | Take count |
| `--to` | string | — | End date/time (ISO/RFC3339); enforced client-side |

### search

```bash
umbraco logs search
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--around` | string | — | Center timestamp for a strict time window (ISO/RFC3339) |
| `--contains` | string | — | Client-side text contains filter across message, exception, and properties |
| `--correlation-id` | string | — | Client-side correlation/request ID contains filter |
| `--count-by` | string | — | Return counts grouped by level, source, or path |
| `--cursor` | string | — | Pagination cursor returned as nextCursor |
| `--filter-expression` | string | — | Serilog filter expression |
| `--flat` | bool | false | Return stable flat JSON entries with properties as an object |
| `--from` | string | — | Start date/time (ISO/RFC3339); enforced client-side |
| `--level` | string | — | Log level |
| `--minutes` | int | 5 | Minutes before and after --around |
| `--params` | string | — | Search params as JSON (accepted keys: startDate,endDate,skip,take,filterExpression,logLevel) |
| `--path` | string | — | Client-side RequestPath contains filter |
| `--redact` | string | — | Comma-separated redaction modes: emails,secrets,tokens,all |
| `--redact-default` | bool | false | Redact emails, secrets, and tokens from output |
| `--skip` | int | -1 | Skip count |
| `--source-context` | string | — | Client-side SourceContext contains filter |
| `--take` | int | -1 | Take count |
| `--to` | string | — | End date/time (ISO/RFC3339); enforced client-side |

### tail

```bash
umbraco logs tail
```

The Management API has no streaming endpoint, so tail polls the log-viewer
with a moving startDate cursor, deduplicates boundary entries, and prints
each new entry exactly once: NDJSON (one JSON object per line) for json
output, one formatted line per entry otherwise. Runs until interrupted or
--for elapses; exits 0 on both.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--contains` | string | — | Only entries containing this substring anywhere |
| `--correlation-id` | string | — | Only entries with this correlation/request id |
| `--filter-expression` | string | — | Serilog filter expression (server-side) |
| `--flat` | bool | false | Flatten entries (timestamp, level, message, sourceContext, ...) |
| `--for` | duration | 0s | Stop after this duration (0 = run until interrupted); exits 0 |
| `--interval` | duration | 2s | Poll interval |
| `--level` | string | — | Only entries at this level (Verbose, Debug, Information, Warning, Error, Fatal) |
| `--path` | string | — | Only entries whose RequestPath contains this value |
| `--redact` | string | — | Redact matching value kinds: emails,secrets,tokens (comma-separated) |
| `--redact-default` | bool | false | Apply the default redaction set (emails, secrets, tokens) |
| `--since` | string | — | Start from this timestamp (RFC3339); default: now (only new entries) |
| `--source-context` | string | — | Only entries whose SourceContext contains this value |

### templates

```bash
umbraco logs templates
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--from` | string | — | Start date (ISO) |
| `--skip` | int | -1 | Skip count |
| `--take` | int | -1 | Take count |
| `--to` | string | — | End date (ISO) |

## Discovering Commands

```bash
# Browse subcommands
umbraco logs --help

# Inspect a specific endpoint schema
umbraco schema logs.<method>
```
