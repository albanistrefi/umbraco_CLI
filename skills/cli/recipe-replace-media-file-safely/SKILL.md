---
name: recipe-replace-media-file-safely
description: "Inspect a media item, see which content uses it, swap its file with a backup, verify, and undo if needed."
metadata:
  version: 0.4.13
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-media
---

# Replace the File Behind a Media Item (Reversibly)

> **PREREQUISITE:** Load the following skills: umbraco-media

Inspect a media item, see which content uses it, swap its file with a backup, verify, and undo if needed.

## Steps

1. `umbraco media inspect <media-id> --output json`
2. `umbraco media references <media-id> --output json`
3. `umbraco media replace-file <media-id> ./new-logo.svg --backup --dry-run --output json`
4. `umbraco media replace-file <media-id> ./new-logo.svg --backup --output json`
5. `umbraco media inspect <media-id> --output json`
6. `umbraco media restore-backup ./media-<media-id>-<timestamp>.backup.json --output json   # only if the result is wrong`

## Tips

- Never PUT /media/{id} by hand to change a file: a temporaryFileId that does not resolve is accepted with 200 and empties the item. replace-file verifies the item afterwards.
- --backup also saves the current binary; Umbraco deletes the replaced file, so an entity-only backup cannot bring it back.
- Use `media download <id> <path>` to pull the current file before editing it locally.

