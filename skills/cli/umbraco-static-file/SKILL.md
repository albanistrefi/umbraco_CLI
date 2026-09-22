---
name: umbraco-static-file
description: "Static file operations"
metadata:
  version: 0.4.21
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# static-file

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco static-file <command> [flags]
```

## Overview

```text
Static file operations (read only). Static files are served from disk; the Management API exposes no write side for them.

Task → command:
  See the folders and files at the root            static-file list
  See what is inside a folder                      static-file children /wwwroot
  Look one path up (name, folder, isFolder)        static-file get /wwwroot/favicon.ico
```

## Read Commands

| Command | Description |
|---------|-------------|
| `static-file children <path>` | List static files and folders inside a folder (paginated; --skip/--take/--all) |
| `static-file get <path>` | Get a static file entry by path |
| `static-file list` | List static files and folders at the root (paginated; --skip/--take/--all) |

### children

```bash
umbraco static-file children <path>
```

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

### get

```bash
umbraco static-file get <path>
```

Looks up one static file by its path. The Management API returns the entry (name, path, parent, isFolder) — static file content is not served through it.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### list

```bash
umbraco static-file list
```

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
umbraco static-file --help

# Inspect a specific endpoint schema
umbraco schema static-file.<method>
```
