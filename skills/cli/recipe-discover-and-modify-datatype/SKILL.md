---
name: recipe-discover-and-modify-datatype
description: "Search for a data type, inspect it, and update its configuration."
metadata:
  version: 0.3.16
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-datatype
---

# Discover and Modify a Data Type

> **PREREQUISITE:** Load the following skills: umbraco-datatype

Search for a data type, inspect it, and update its configuration.

## Steps

1. `umbraco datatype search --query "rich text" --output json`
2. `umbraco datatype get <id> --output json`
3. `umbraco datatype extensions <id> --output json`
4. `umbraco datatype add-extension <id> My.Custom.Extension --dry-run --output json`
5. `umbraco datatype add-extension <id> My.Custom.Extension --output json`

## Tips

- Use `datatype extensions` to see the current extension list before modifying.
- add-extension and remove-extension handle fetch-merge-write automatically.

