---
name: umbraco-tree
description: "Tree navigation helpers"
metadata:
  version: 0.2.6
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# tree

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco tree <command> [flags]
```

## Read Commands

| Command | Description |
|---------|-------------|
| `tree walk <path>` | Resolve a content tree path like Home/Partners/Partner List to a node ID |

### walk

```bash
umbraco tree walk <path>
```

## Discovering Commands

```bash
# Browse subcommands
umbraco tree --help

# Inspect a specific endpoint schema
umbraco schema tree.<method>
```
