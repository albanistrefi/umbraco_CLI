---
name: umbraco-logs
description: "Log and diagnostics operations"
metadata:
  version: 0.4.1
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
| `logs level-count` | Get count per level |
| `logs list` | List log entries |
| `logs search` | Search logs |
| `logs templates` | List paginated log message templates |

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
| `--filter-expression` | string | — | Serilog filter expression |
| `--from` | string | — | Start date (ISO) |
| `--level` | string | — | Log level |
| `--params` | string | — | Filter params as JSON (accepted keys: startDate,endDate,skip,take,filterExpression,logLevel) |
| `--skip` | int | -1 | Skip count |
| `--take` | int | -1 | Take count |
| `--to` | string | — | End date (ISO) |

### search

```bash
umbraco logs search
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--filter-expression` | string | — | Serilog filter expression |
| `--from` | string | — | Start date (ISO) |
| `--level` | string | — | Log level |
| `--params` | string | — | Search params as JSON (accepted keys: startDate,endDate,skip,take,filterExpression,logLevel) |
| `--skip` | int | -1 | Skip count |
| `--take` | int | -1 | Take count |
| `--to` | string | — | End date (ISO) |

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
