# Changelog

## v0.2.5 - 2026-03-27

- changed the default `document publish` and save-and-publish payload to use the invariant publish schedule `{"publishSchedules":[{"culture":null}]}` when no explicit publish flags are provided

## v0.2.4 - 2026-03-27

- fixed `document update --merge-json` and other merge-based update flows so merged payloads built from fetched server content are not rejected by local input validation when existing content contains control characters

## v0.2.3 - 2026-03-27

- fixed the document tree commands to prefer the Umbraco v17 tree endpoints for `root`, `children`, and `ancestors`
- added property-level document updates and a `--save-and-publish` workflow
- added `media search` with compatibility-aware routing
- added `tree walk` to resolve content paths like `Home/Partners/Partner List` to node IDs
- added `document csv-update` for row-driven batch content updates from CSV files
- added persistent `auth login`, `auth status`, and `auth logout` commands backed by `~/.umbraco/config.json`

## v0.2.2 - 2026-03-26

- fixed `template`, `doctype`, and `server` route mappings to prefer the Management API routes used by current Umbraco versions while keeping compatibility fallbacks
- updated `umbraco schema ...` output so the advertised primary routes match the corrected endpoint mappings
- improved auth and connectivity errors to show the resolved base URL and token endpoint
- added support for project-local `.umbraco-cli.env` files for CLI-specific base URL and credential setup
- improved bounded base URL auto-discovery for adjacent/local Umbraco web-host projects while still rejecting ambiguous candidates
- fixed `document search` to prefer the v17-compatible `/item/document/search` route with fallback support
- added `document search --under <parent-id>` for first-class subtree-scoped content discovery
- added `document update --merge-json` and `document bulk-update` for safer repeated content updates
- fixed the bundled skill metadata so `npm run verify:skills` now passes with the current 67-skill bundle
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
