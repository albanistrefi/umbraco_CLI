---
name: umbraco-health
description: "Health check operations"
metadata:
  version: 0.4.15
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# health

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco health <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `health group <name>` | Get health check group details |
| `health groups` | List health check groups |
| `health run <group-name>` | Run health checks for group |

### group

```bash
umbraco health group <name>
```

### groups

```bash
umbraco health groups
```

### run

```bash
umbraco health run <group-name>
```

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `health action <id>` | Execute a health check action |

### action

```bash
umbraco health action <id>
```

POST /health-check/execute-action. On current servers <id> is the health check id from 'health run' results and fills healthCheck.id in the body when --json omits it. On older servers (which 404 the modern route) <id> must be the legacy action id — it is forwarded to POST /health-check/{actionId} with the --json payload unchanged.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Action payload as JSON |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco health action <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco health action <id> [flags]
```

## Discovering Commands

```bash
# Browse subcommands
umbraco health --help

# Inspect a specific endpoint schema
umbraco schema health.<method>
```
