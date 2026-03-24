# Changelog

## v0.2.2 - 2026-03-24

- fixed `template`, `doctype`, and `server` route mappings to prefer the Management API routes used by current Umbraco versions while keeping compatibility fallbacks
- updated `umbraco schema ...` output so the advertised primary routes match the corrected endpoint mappings
- improved auth and connectivity errors to show the resolved base URL and token endpoint
- added support for project-local `.umbraco-cli.env` files for CLI-specific base URL and credential setup
- updated docs for the local CLI config workflow

## v0.2.1 - 2026-03-13

- fixed release automation so GoReleaser uses a dedicated token for cross-repo Homebrew tap updates
- bumped patch version after the `v0.2.0` Homebrew publish failure

## v0.2.0 - 2026-03-13

- fixed datatype discovery commands to use compatibility-aware Management API routes
- added richer API endpoint error messages with resolved method/path hints
- added `datatype update --merge-json` for fetch-merge-write partial updates
- added `datatype extensions`, `datatype add-value`, `datatype remove-value`, `datatype add-extension`, and `datatype remove-extension`
- added layered config loading from env, project config, project `.env`, user config, and local `.NET` URL discovery
- updated docs and examples for the new datatype and config workflows
