# Changelog

## v0.3.0 - 2026-05-08

- added agent-focused create ergonomics: generated IDs, minimal create responses, `--print-template`, and `umbraco schema <endpoint> --template`
- added `media upload`, `document copy --publish`, `auth status --check`, datatype `--editor-alias`, and compact collection output flags
- taught successful empty-body create/copy responses to surface IDs from `Location` headers
- regenerated CLI skills so the generated command references include the new flags and commands

## v0.2.9 - 2026-05-07

- normalized `auth login --base-url` through the shared config base URL rules so `/umbraco` suffixes do not create duplicated token URLs
- changed root command initialization to return CLI errors instead of panicking when runtime/config loading fails
- added schema coverage tests that fail when registered direct API commands are missing `umbraco schema` entries

## v0.2.8 - 2026-05-05

- fixed `doctype add-property --container` so it looks up containers by name (the canonical Umbraco field) instead of the non-existent alias field; the v0.2.7 lookup never matched against real backoffice payloads
- added `umbraco --version` to print the installed CLI release identifier
- added `doctype add-container <id> --name --type Tab|Group [--parent <name>]` convenience command that resolves an optional parent container by name, normalizes type casing, and rejects duplicate names
- centralized the CLI version on `internal/version/VERSION` (Go embeds it at build, `npm run sync:version` propagates it to `package.json`/`package-lock.json`, and `npm run verify:skills` blocks releases when those files or the CHANGELOG drift)
- renamed the internal `mergeDatatypePayload` helper to `mergeAliasPayload` since it now serves document, doctype, and datatype merge flows

## v0.2.7 - 2026-05-05

- added `doctype update --merge-json` for partial document type updates that fetch the current schema, deep-merge the patch (including alias-keyed `properties` and `containers` arrays), and PUT the merged payload with validation skipped
- added `doctype add-property <id> --alias --name --data-type --container` convenience command that resolves an existing tab/group container alias to its ID, generates a v4 property ID, and rejects duplicate aliases or unknown containers before writing
- aliased the `doctype` command group as `document-type` so `umbraco document-type ...` matches the underlying Management API path

## v0.2.6 - 2026-04-08

- added `umbraco generate-skills` for self-documenting CLI skills generated from the cobra command tree, plus the bundled CLI skills under `skills/cli/` and the `verify-skills` script update that backs them

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
