---
name: umbraco-logs
description: "Log and diagnostics operations"
metadata:
  version: 0.3.5
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
| `logs levels` | List log levels |
| `logs list` | List log entries |
| `logs search` | Search logs |
| `logs templates` | List log templates |

### level-count

```bash
umbraco logs level-count
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--from` | string | — | Start date (ISO) |
| `--params` | string | — | Filter params as JSON |
| `--to` | string | — | End date (ISO) |

### levels

```bash
umbraco logs levels
```

### list

```bash
umbraco logs list
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--from` | string | — | Start date (ISO) |
| `--level` | string | — | Log level |
| `--params` | string | — | Filter params as JSON |
| `--skip` | int | -1 | Skip count |
| `--take` | int | -1 | Take count |
| `--to` | string | — | End date (ISO) |

### search

```bash
umbraco logs search
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--filter-expression` | string | — | Filter expression |
| `--params` | string | — | Search params as JSON |
| `--skip` | int | -1 | Skip count |
| `--take` | int | -1 | Take count |

### templates

```bash
umbraco logs templates
```

## Discovering Commands

```bash
# Browse subcommands
umbraco logs --help

# Inspect a specific endpoint schema
umbraco schema logs.<method>
```
