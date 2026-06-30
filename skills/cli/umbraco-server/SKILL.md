---
name: umbraco-server
description: "Server information and diagnostics"
metadata:
  version: 0.4.6
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# server

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco server <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `server config` | Get server config |
| `server info` | Get server info |
| `server status` | Get server status |
| `server troubleshoot` | Run troubleshooting checks |
| `server upgrade-check` | Check upgrade readiness |

### config

```bash
umbraco server config
```

### info

```bash
umbraco server info
```

### status

```bash
umbraco server status
```

### troubleshoot

```bash
umbraco server troubleshoot
```

### upgrade-check

```bash
umbraco server upgrade-check
```

## Discovering Commands

```bash
# Browse subcommands
umbraco server --help

# Inspect a specific endpoint schema
umbraco schema server.<method>
```
