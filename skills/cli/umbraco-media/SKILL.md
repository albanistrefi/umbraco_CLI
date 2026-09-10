---
name: umbraco-media
description: "Media asset operations"
metadata:
  version: 0.4.15
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-shared
---

# media

> **PREREQUISITE:** Read `../umbraco-shared/SKILL.md` for auth, global flags, and security rules.

```bash
umbraco media <command> [flags]
```

## Overview

```text
Media asset operations.

Task → command:
  Which file is behind this item, how big, what dimensions?   media inspect <id>
  Which content uses this media item?                          media references <id>   (aliases: find-references, referenced-by, usage)
  Is any of these items referenced at all?                     media are-referenced <id> [<id>...]
  Download the file to disk                                    media download <id> <path>
  Upload a new file as a new media item                        media upload <file> --type <alias>
  Swap the file behind an existing item (keep other values)    media replace-file <id> <file> --backup
  Undo a bad write                                             media restore-backup <backup.json>
  Change name/values without touching the file                 media update <id> --merge-json '{...}' --backup
  Where is it served from?                                     media urls <id>
```

## Read Commands

| Command | Description |
|---------|-------------|
| `media are-referenced` | Bulk check: which of these media IDs are referenced by something |
| `media bin children <id>` | List children of a trashed media item |
| `media bin list` | List media items at the recycle bin root |
| `media bin original-parent <id>` | Get the original parent of a trashed media item (the default restore target) |
| `media children <id>` | Get child media items (paginated; --skip/--take/--all) |
| `media get <id>` | Get media by ID |
| `media inspect <id>` | Summarize a media item: name, type, file src/URL, extension, size, dimensions |
| `media referenced-descendants <id>` | List items that reference this media item or any of its descendants |
| `media references <id>` | List items that reference this media item (paginated; --skip/--take/--all) |
| `media root` | Get root media items (paginated; --skip/--take/--all) |
| `media search` | Search media items |
| `media urls <id>` | Get media URLs |

### are-referenced

```bash
umbraco media are-referenced
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ids` | string | — | Comma-separated media GUIDs to check (required) |

### bin children

```bash
umbraco media bin children <id>
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

### bin list

```bash
umbraco media bin list
```

GET /recycle-bin/media/root. Paginated; use 'bin children <id>' to descend into trashed subtrees.

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

### bin original-parent

```bash
umbraco media bin original-parent <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### children

```bash
umbraco media children <id>
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
umbraco media get <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fields` | string | — | Limit response fields (comma-separated top-level keys) |

### inspect

```bash
umbraco media inspect <id>
```

Aliases: `info`

One-call view of what a media item points at. Combines GET /media/{id} with the public URL and flattens the file property (default umbracoFile) into file.src, file.url, file.extension, file.bytes. Raster dimensions come from the umbracoWidth/umbracoHeight values; for SVGs the file is fetched and its viewBox/width/height attributes are reported (skip with --no-fetch). Use before and after 'media replace-file' to confirm the swap, or 'media references <id>' to see which content uses the item.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--culture` | string | — | Culture of the file value to summarize (required when the property varies by culture) |
| `--no-fetch` | bool | false | Do not download SVG files to read their viewBox |
| `--property` | string | umbracoFile | File property alias to summarize |
| `--segment` | string | — | Segment of the file value to summarize |

### referenced-descendants

```bash
umbraco media referenced-descendants <id>
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

### references

```bash
umbraco media references <id>
```

Aliases: `find-references`, `referenced-by`, `usage`

Wraps GET /media/{id}/referenced-by. Same content-audit role as 'document references' for media assets — answers "which content uses this media item?".

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

### root

```bash
umbraco media root
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

### search

```bash
umbraco media search
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--params` | string | — | Search parameters as JSON; convenience flags fill in missing keys, --params wins on collisions |
| `--query` | string | — | Search query |
| `--skip` | int | -1 | Skip count (passes through as ?skip=N; lets you walk past the server page size on large children/root collections) |
| `--take` | int | -1 | Take count (passes through as ?take=N; combine with --skip to page) |

### urls

```bash
umbraco media urls <id>
```

Aliases: `url`

## Mutation Commands

> **Safety:** Always use `--dry-run` first. Remove the flag only after verifying the dry-run output.

| Command | Description |
|---------|-------------|
| `media bin delete <id>` | Permanently delete one media item from the recycle bin |
| `media bin empty` | Permanently delete everything in the media recycle bin |
| `media create` | Create media from JSON payload |
| `media create-folder [name]` | Create media folder |
| `media download <id> <path>` | Download the file behind a media item to a local path |
| `media move <id>` | Move media item |
| `media replace-file <id> <file>` | Replace the file behind an existing media item, keeping its other values |
| `media restore <id>` | Restore a media item from the recycle bin |
| `media restore-backup <file>` | Restore a media item from a --backup JSON file |
| `media sort` | Reorder sibling media items into an explicit order |
| `media sort-children [parent-id]` | Sort all children of a node by a field |
| `media trash <id>` | Move media item to recycle bin |
| `media update <id>` | Update media item |
| `media upload <file>` | Upload a file and create a media item |

### bin delete

```bash
umbraco media bin delete <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm permanent deletion |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco media bin delete <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco media bin delete <id> --force [flags]
```

### bin empty

```bash
umbraco media bin empty
```

DELETE /recycle-bin/media. Destroys every trashed media item; there is no undo.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--force` | bool | false | Confirm emptying the recycle bin |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco media bin empty [flags] --dry-run

# 2. Execute with the same flags
umbraco media bin empty --force [flags]
```

### create

```bash
umbraco media create
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Create payload as JSON |
| `--print-template` | bool | false | Print an annotated JSON skeleton; substitute placeholders before passing to --json |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco media create [flags] --dry-run

# 2. Execute with the same flags
umbraco media create [flags]
```

### create-folder

```bash
umbraco media create-folder [name]
```

Folders are regular media items of the built-in Folder type, so this resolves the Folder media type and POSTs /media with a variants envelope. --json passes a full media create payload through verbatim.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full media create payload as JSON (bypasses Folder-type resolution) |
| `--parent` | string | — | Target parent media ID (omit for a root-level folder) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco media create-folder [name] [flags] --dry-run

# 2. Execute with the same flags
umbraco media create-folder [name] [flags]
```

### download

```bash
umbraco media download <id> <path>
```

Aliases: `get-file`

Resolves the file property (default umbracoFile) and fetches the asset from the same host, writing it verbatim. If <path> is an existing directory (or ends with /), the server-side file name is used inside it. An existing file at the destination is overwritten; --dry-run reports the resolved destination without fetching or writing.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--culture` | string | — | Culture of the file value to download (required when the property varies by culture) |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--property` | string | umbracoFile | File property alias to download |
| `--segment` | string | — | Segment of the file value to download |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco media download <id> <path> [flags] --dry-run

# 2. Execute with the same flags
umbraco media download <id> <path> [flags]
```

### move

```bash
umbraco media move <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Action payload as JSON |
| `--to` | string | — | Target parent ID shortcut for {"target":{"id":...}} |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco media move <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco media move <id> [flags]
```

### replace-file

```bash
umbraco media replace-file <id> <file>
```

Aliases: `replace`

Uploads <file> as a temporary file, rewrites the file property (default umbracoFile) on the existing item, and verifies the result. Every other value is preserved; the server recomputes derived values (umbracoBytes, umbracoExtension, dimensions).

After the write the item is fetched again; if the server accepted the PUT but left the item with no values (what happens when a temporary file id does not resolve) the command fails instead of reporting success. Pass --backup to save the pre-change item first; 'media restore-backup <file>' puts it back.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--backup` | string | — | Save the current item to a JSON file before writing; bare --backup writes ./<collection>-<id>-<timestamp>.backup.json, --backup=<path> chooses the file. Undo with '<collection> restore-backup <file>' where available |
| `--culture` | string | — | Culture of the file value to replace (required when the property varies by culture) |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--name` | string | — | Also rename the media item |
| `--property` | string | umbracoFile | File property alias to replace |
| `--segment` | string | — | Segment of the file value to replace |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco media replace-file <id> <file> [flags] --dry-run

# 2. Execute with the same flags
umbraco media replace-file <id> <file> [flags]
```

### restore

```bash
umbraco media restore <id>
```

PUT /recycle-bin/media/{id}/restore. The restore target defaults to the item's original parent (looked up via the recycle-bin API); pass --to for a different parent, or --to root to restore at the media root.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--to` | string | — | Restore target parent ID, or 'root' (defaults to the original parent) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco media restore <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco media restore <id> [flags]
```

### restore-backup

```bash
umbraco media restore-backup <file>
```

Reads a file written by 'media replace-file --backup' or 'media update --backup' and PUTs the saved item back to the server, then verifies the item is no longer empty. Backups from replace-file also carry the original file: Umbraco deletes the previous file when it is replaced, so the restore re-uploads the saved binary rather than pointing at a path that no longer exists. Backups from 'media update --backup' are metadata-only; the restore first checks the referenced file is still served and refuses to write if it is gone. This restores values and name; it does not undo a move or delete (use 'media restore' for the recycle bin).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco media restore-backup <file> [flags] --dry-run

# 2. Execute with the same flags
umbraco media restore-backup <file> [flags]
```

### sort

```bash
umbraco media sort
```

PUT /media/sort. Pass --ids with the desired order (sortOrder is assigned from position) and --parent for the common parent; omit --parent when sorting root-level items. IDs not listed keep their relative order after the sorted ones.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |
| `--ids` | string | — | Comma-separated GUIDs in the desired order |
| `--json` | string | — | Sort payload as JSON |
| `--parent` | string | — | Parent ID (omit for root-level items) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco media sort [flags] --dry-run

# 2. Execute with the same flags
umbraco media sort [flags]
```

### sort-children

```bash
umbraco media sort-children [parent-id]
```

PUT /media/root/sort-children or /media/{id}/sort-children (Umbraco 18.1+). Reorders every child of the parent (root when [parent-id] is omitted) server-side by --field. For explicit manual ordering use 'media sort' instead.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--direction` | string | Ascending | Sort direction: Ascending or Descending (asc/desc accepted) |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--field` | string | — | Sort field: Name, CreateDate, or UpdateDate (required) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco media sort-children [parent-id] [flags] --dry-run

# 2. Execute with the same flags
umbraco media sort-children [parent-id] [flags]
```

### trash

```bash
umbraco media trash <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Print the planned request without executing |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco media trash <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco media trash <id> [flags]
```

### update

```bash
umbraco media update <id>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--backup` | string | — | Save the current item to a JSON file before writing; bare --backup writes ./<collection>-<id>-<timestamp>.backup.json, --backup=<path> chooses the file. Undo with '<collection> restore-backup <file>' where available |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--json` | string | — | Full replacement payload as JSON (fields not mentioned are reset by the server) |
| `--merge-json` | string | — | Partial JSON deep-merged into the current resource before update (fields not mentioned are preserved) |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco media update <id> [flags] --dry-run

# 2. Execute with the same flags
umbraco media update <id> [flags]
```

### upload

```bash
umbraco media upload <file>
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--culture` | string | — | Culture code for culture-varying media types |
| `--dry-run` | bool | false | Print the planned request without executing |
| `--name` | string | — | Media item name (defaults to file name without extension) |
| `--parent` | string | — | Target parent media ID |
| `--property` | string | umbracoFile | File property alias |
| `--type` | string | — | Media type id, alias, or name |

**Safe pattern:**

```bash
# 1. Rehearse with the exact flags you will execute with
umbraco media upload <file> [flags] --dry-run

# 2. Execute with the same flags
umbraco media upload <file> [flags]
```

## Discovering Commands

```bash
# Browse subcommands
umbraco media --help

# Inspect a specific endpoint schema
umbraco schema media.<method>
```
