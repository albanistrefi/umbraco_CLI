---
name: recipe-search-and-publish-documents
description: "Search for documents matching a query and publish them."
metadata:
  version: 0.2.8
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-document
---

# Search and Publish Documents

> **PREREQUISITE:** Load the following skills: umbraco-document

Search for documents matching a query and publish them.

## Steps

1. `umbraco document search --query "draft" --output json`
2. `umbraco document publish <id> --json '{"publishSchedules":[{"culture":null}]}' --dry-run --output json`
3. `umbraco document publish <id> --json '{"publishSchedules":[{"culture":null}]}' --output json`

## Tips

- Use --under <parent-id> to scope search to a subtree.
- Publish each document individually after reviewing the search results.

