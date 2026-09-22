---
name: umbraco-relation
description: "Relations and relation types"
metadata:
  version: 0.4.21
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# relation

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco relation <command> [flags]
```

## Overview

```text
Relations and relation types.

Task → command:
  Which relation types exist?                          relation type list
  What does one relation type do (bidirectional?)      relation type get <id>
  Resolve relation type GUIDs to names in one call     relation type items --ids <id1,id2>
  What relations are stored for a relation type?       relation list --type <id> --take 5
  Walk every relation of a type                        relation list --type <id> --all
  Only show the two ends of each relation              relation list --type <id> --fields parent,child

The Management API exposes relations per relation type only — there is no
by-parent or by-child read — so start from 'relation type list', then filter
the rows of 'relation list --type <id>' client-side.
```

## Read Commands

| Command | Description |
|---------|-------------|
| `relation list` | List the relations stored for one relation type (paginated; --skip/--take/--all) |
| `relation type get <id>` | Get one relation type (alias, direction, tracked object types) |
| `relation type items` | Resolve relation type GUIDs to names in one call |
| `relation type list` | List relation types (paginated; --skip/--take/--all) |

### list

```bash
umbraco relation list
```

GET /relation/type/{id}. The relation type GUID comes from 'relation type list'. Each row names the two ends of the relation (parent/child) — the API has no by-parent or by-child endpoint, so narrow the rows with --fields or --all plus client-side filtering. Bookkeeping types such as umbMedia can hold tens of thousands of rows, so keep --take small before reaching for --all.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |
| `--type` | string | — | Relation type GUID whose relations to list (required) |

### type get

```bash
umbraco relation type get <id>
```

GET /relation-type/{id}. isBidirectional tells you whether the relation is walked from both ends; isDependency tells you whether deleting one end is blocked by the other.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### type items

```bash
umbraco relation type items
```

GET /item/relation-type?id=…. The item read for relation types: pass the GUIDs seen in other payloads and get their names back without one request per ID.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--ids` | string | — | Comma-separated relation type GUIDs (required) |

### type list

```bash
umbraco relation type list
```

GET /relation-type. The catalogue of relation kinds the instance knows about (document/media pickers, tracked references, recycle-bin bookkeeping). The id of a row is the --type argument of 'relation list'.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | false | Walk every page until exhausted (auto-paginates with --take as the page size, default 500; combine with --skip to start partway through). Bounded by an internal 100k-item ceiling. |
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |
| `--first-n` | int | 0 | Return only the first N items from item collections |
| `--ids-only` | bool | false | Return only item IDs for item collections |
| `--params` | string | — | Query parameters as JSON |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--summarize` | bool | false | Return only id/name/alias fields for item collections |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

## Discovering Commands

```bash
# Browse subcommands
umbraco relation --help

# Inspect a specific endpoint schema
umbraco schema relation.<method>
```
