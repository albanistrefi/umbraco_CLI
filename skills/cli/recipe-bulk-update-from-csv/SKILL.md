---
name: recipe-bulk-update-from-csv
description: "Use csv-update to batch-modify a property across multiple documents."
metadata:
  version: 0.4.4
  requires:
    bins:
      - umbraco
    skills:
      - umbraco-document
---

# Bulk Update Documents from CSV

> **PREREQUISITE:** Load the following skills: umbraco-document

Use csv-update to batch-modify a property across multiple documents.

## Steps

1. `umbraco document csv-update --file partners.csv --property skills --dry-run --output json`
2. `umbraco document csv-update --file partners.csv --property skills --output json`

## Tips

- The CSV must have an `id` column with document UUIDs and a column matching the --property alias.
- Dry-run output shows what would change without modifying anything.

