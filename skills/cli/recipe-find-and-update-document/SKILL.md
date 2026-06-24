---
name: recipe-find-and-update-document
description: "Walk the content tree to find a document, update a property, and publish."
metadata:
  version: 0.4.5
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-tree
      - umbraco-document
---

# Find and Update a Document by Path

> **PREREQUISITE:** Load the following skills: umbraco-tree, umbraco-document

Walk the content tree to find a document, update a property, and publish.

## Steps

1. `umbraco tree walk "Home/Partners/Partner List" --output json`
2. `umbraco document get <id-from-step-1> --fields "id,name,values" --output json`
3. `umbraco document update <id> --property skills --value "C#;Go" --dry-run --output json`
4. `umbraco document update <id> --property skills --value "C#;Go" --output json`
5. `umbraco document publish <id> --json '{"publishSchedules":[{"culture":null}]}' --dry-run --output json`
6. `umbraco document publish <id> --json '{"publishSchedules":[{"culture":null}]}' --output json`

## Tips

- Always dry-run mutations before executing.
- Use --fields on reads to keep responses small.
- Reuse IDs returned by API responses; never construct them manually.

