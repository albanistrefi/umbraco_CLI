---
name: umbraco-models-builder
description: "Trigger and inspect ModelsBuilder source generation"
metadata:
  version: 0.4.2
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
| `models-builder dashboard` | Get dashboard: mode, modelsNamespace, outOfDate flag, last error |
| `models-builder status` | Get out-of-date status: Current | OutOfDate | Unknown |

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

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `models-builder build` | Trigger source generation (SourceCodeManual / SourceCodeAuto only) |

### build

```bash
umbraco models-builder build
```

POSTs to /models-builder/build. Pre-checks the dashboard mode so non-source-generating modes (InMemory, Nothing) fail with a clear message instead of an opaque server error. With --wait, polls status until Current or --timeout elapses. --dry-run runs the dashboard/mode pre-checks and returns the planned POST without triggering generation.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Run dashboard/mode pre-checks and return the planned POST without triggering generation; incompatible with --wait |
| `--poll-interval` | duration | 1s | How often to poll status when --wait is set |
| `--timeout` | duration | 1m0s | How long to wait when --wait is set (e.g. 30s, 2m) |
| `--wait` | bool | false | Poll status after triggering the build until it reports Current or --timeout elapses |

**Safe pattern:**

```bash
# 1. Dry run first
umbraco models-builder build --dry-run

# 2. Execute
umbraco models-builder build
```

## Discovering Commands

```bash
# Browse subcommands
umbraco models-builder --help

# Inspect a specific endpoint schema
umbraco schema models-builder.<method>
```
