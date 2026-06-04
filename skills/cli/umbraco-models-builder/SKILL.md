---
name: umbraco-models-builder
description: "Trigger and inspect ModelsBuilder source generation"
metadata:
  version: 0.3.14
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# models-builder

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco models-builder <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `models-builder build` | Trigger source generation (SourceCodeManual / SourceCodeAuto only) |
| `models-builder dashboard` | Get dashboard: mode, modelsNamespace, outOfDate flag, last error |
| `models-builder status` | Get out-of-date status: Current | OutOfDate | Unknown |

### build

```bash
umbraco models-builder build
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--poll-interval` | duration | 1s | How often to poll status when --wait is set |
| `--timeout` | duration | 1m0s | How long to wait when --wait is set (e.g. 30s, 2m) |
| `--wait` | bool | false | Poll status after triggering the build until it reports Current or --timeout elapses |

### dashboard

```bash
umbraco models-builder dashboard
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields |

### status

```bash
umbraco models-builder status
```

## Discovering Commands

```bash
# Browse subcommands
umbraco models-builder --help

# Inspect a specific endpoint schema
umbraco schema models-builder.<method>
```
