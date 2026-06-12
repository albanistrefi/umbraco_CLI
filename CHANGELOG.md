# Changelog

## v0.4.3 - 2026-06-12

Fixes for three field reports from agent runs, all reproduced live before fixing:

- fixed `tree walk` failing with `could not find` on paths that plainly exist: modern tree responses carry document names inside `variants[]` with no top-level `name` field, so the matcher silently found nothing. Names now match against the top-level field (older servers) and every variant name
- added `--resolve-doctype` to `document root` and `document children`: tree responses carry only `{id, icon}` for an item's document type, so agents couldn't reason about content types without per-item lookups. The flag annotates each item's `documentType` with its alias, fetching each distinct type exactly once
- fixed `datatype create --json` silently dropping a `configuration` map: the API accepts only a `values` array and ignores the unknown key — and the CLI's own `--print-template` taught the wrong shape. `configuration` now converts to `values` automatically (deterministic, alias-sorted) on `datatype create` and `update`, payloads mixing both shapes are rejected, and the template teaches `values` with the required `editorUiAlias`
- hardened the datatype merge path against non-standard responses: a `configuration` map surviving into a merged update body is folded into `values` (the patch deep-merged over legacy settings per key) and the key never reaches the PUT. No supported Management API returns that shape (verified against the v14.0-era and v17 specs); this keeps the CLI's tolerance of it consistent
- update normalization split into input hooks (may reject, e.g. the mixed-shape error) and post-merge hooks (output hygiene, e.g. the Automate response-field strips), so input conveniences never fire on server-echoed fields

## v0.4.2 - 2026-06-11

### Umbraco Automate support (53 commands)

- added the `automate` command group covering the full Automate Management API, timed with the product's public launch at Codegarden. Requires Umbraco Automate on the target instance
- **catalogue discovery**: `actions`, `triggers`, `step-types`, `connection-types`, `control-flows`, `notification-channels`, `webhook-authenticators`, and `output-schema <alias>` for resolving dynamic step output schemas used in `${...}` bindings. Catalogue responses embed full JSON schemas, so the commands support `--fields` projection
- **automation authoring**: `create`/`update`/`delete`, the `publish`/`unpublish`/`re-enable` lifecycle, and `ancestors`. `update --merge-json` picks up the optimistic-concurrency `version` automatically and strips the response-only fields the update model rejects
- **export → validate → import round-trip**: `automation validate` checks a definition server-side without writing anything (the authoring dry-run); `import` creates from an export model, `import-update` overwrites an existing automation
- **run control**: `run get/replay/resume/suspend/terminate`; **approvals**: `pending` and `decide`; **metrics**: `summary` and `by-automation`
- **workspaces** (with nested automation `group` management) and **connections** (with `connection test` for verifying credentials against the external service)
- **version history**: `list`/`get`/`compare`/`rollback` for automations, workspaces, and connections — the undo path for agent edits
- the schema generator now reads multiple vendored OpenAPI documents (core + Automate); `umbraco schema automate.*` entries carry the Automate mount as `apiRoot`

### Generated skills

- nested subgroups now document as full commands instead of empty stubs — `document version rollback`, `document domains set`, `user client-credentials create`, and the whole `automate` tree previously rendered as bare group names in the bundled skills
- added `generate-skills --include-hidden --filter <name>` for generating private docs of hidden command groups

## v0.4.1 - 2026-06-10

Fixes for the three review findings on the v0.4.0 pull requests:

- fixed `document restore` dead-ending on servers without the recycle-bin API: a 404 from the original-parent lookup now proceeds with a null target so the modern→legacy restore fallback chain decides, instead of erroring before the legacy `POST /document/{id}/restore` could run
- fixed `api.JoinPath` leaving dot-only path arguments unescaped — `url.PathEscape` treats dots as unreserved, so a literal `.` or `..` argument still produced a relative-path segment that proxies and servers normalize into a route rewrite. Dot-only segments are now percent-encoded (`..` arrives as `%2E%2E`)
- fixed `document update` masking fetch failures and invalid-JSON errors behind the `requires exactly one of --json, --merge-json, or --property` message; the mode check runs up front and real errors propagate unchanged

## v0.4.0 - 2026-06-10

### Breaking changes

- **`--json` is now a full replacement and `--merge-json` a fetch-and-merge on every update command.** `datatype update --json` and `member update --json` previously fetch-and-merged; pass `--merge-json` for partial edits there now. `media update` and `template update` gain `--merge-json`. The uniform contract: `--json` = the complete intended state (the server resets unmentioned fields), `--merge-json` = a patch (unmentioned fields preserved)
- **hard deletes require `--force` or `--dry-run`** on `document/media/doctype/datatype/member/template delete` and `user client-credentials delete`, matching the existing gate on `dictionary delete` and `bulk-update`. `trash` stays ungated — the recycle bin is reversible
- **search convenience flags now merge into `--params`** (with `--params` winning key collisions) instead of being silently ignored when `--params` is set
- **empty 204 successes print `{"<verb>": true}`** (`updated` / `deleted` / `published` / `moved` / `trashed` / …) instead of `null` on every mutation, so scripts can distinguish success from failure. `datatype add-value` / `remove-value` report the same mutation summary as the block commands in all three cases (no-op / dry-run / applied)
- `--dry-run` help text no longer claims server-side validation: it prints the planned request without executing (it never reached the server). `bulk-update`/`csv-update` per-item messages say `planned` instead of `validated`

### Fixed: mutations broken on modern Umbraco (verified live)

- `document/media/doctype move` and `document/media trash` used POST where modern servers serve PUT — `document trash` 404'd on every modern server. Mutations whose method or route moved between API versions now try the modern form first and fall back on 404/405
- `document restore` now uses PUT `/recycle-bin/document/{id}/restore` and resolves the document's original parent as the restore target (`--to <parent-id>` or `--to root` to override); the old POST route never worked on modern servers
- `media urls` moved to `GET /media/urls?id=`; `media create-folder` now creates a media item of the built-in Folder type (`POST /media/folder` does not exist on modern servers)
- `document copy --publish --dry-run` no longer errors — the publish step is planned against a placeholder ID
- `tree walk` pages through all children per segment instead of silently missing nodes beyond the first server page

### New command surfaces (41 commands)

- `document version list/get/rollback/prevent-cleanup` and `document audit-log` — version history, rollback (the undo path for bulk edits), and the change trail
- `webhook list/get/create/update/delete/events/logs` — full webhook management including the delivery log
- `language list/get/create/update/delete/default/cultures` — language CRUD plus ISO-culture discovery for variant content
- `user list/get/create/invite/update/delete/enable/disable/unlock/set-groups/current/permissions` and `user client-credentials list/create/delete` — backoffice user management including the OAuth credentials API users authenticate with; `user permissions` lets an agent check write access before mutating
- `user-group list/get/create/update/delete/add-users/remove-users`
- `document publish-descendants` (+ `publish-descendants-result` for the async task), `document sort`, `document domains get/set`, `document public-access get/set/remove` — `public-access set` resolves create-vs-replace itself

### Input validation inverted

- removed the heuristic body validation that rejected legitimate CMS content: multiline Razor in `template create/update` (impossible before — newlines were "control characters"), values containing `?`/`#`/`%` under property aliases like `video`/`width`, and `%20` anywhere ("pre-encoded"). Request bodies pass through untouched — the Management API is the authority
- the actual injection surface is now covered: every user-supplied path argument is escaped, so `umbraco document get "../server/status"` reaches the server as one literal segment instead of rewriting the route

### Schema introspection generated from the OpenAPI document

- `internal/schema` operation detail is generated from the vendored Management API OpenAPI document (456 operations). `umbraco schema document.update` now reports the real request model (required `values`+`variants`, property types) instead of "Raw JSON payload accepted by the endpoint". CI regenerates and diffs both the schemas and the bundled CLI skills; a test fails any binding that points at an operation the spec doesn't declare — which is how the broken mutations above were found

### Reliability and hygiene

- a malformed line in an unrelated `.env` up the directory tree no longer bricks every invocation; .NET host discovery runs only when no other source supplies a base URL and is best-effort
- `--help`/`--version`/`schema`/`generate-skills` keep working when config resolution fails; the first command that reaches the API reports the real cause
- Ctrl+C cancels in-flight requests, retry sleeps, and the models-builder `--wait` poll loop (`signal.NotifyContext` + per-command contexts)
- `--all` auto-pagination stops re-probing 404ing endpoint fallbacks on every page; fallback chains use `errors.As` so future error wrapping can't break them
- media-type resolution distinguishes "lookup failed" (server down, auth expired) from "no match"
- `datatype list/root`, `doctype list`, `template root` gain `--skip/--take/--all/--params`; `template get` gains client-side `--fields` projection; `dictionary create --json` injects a CLI-generated id like every other create
- `skills-lock.json` hashes are now verified (they were write-only); `bundle:skills` no longer deletes the generated `skills/cli` output; CI runs `gofmt`, `go vet`, `-race`, and `verify:skills`

## v0.3.17 - 2026-06-08

- added `datatype block update <datatypeId> --content-element-type <guid> [flags]` for partial edits to an existing block on a Block List / Block Grid datatype. Only flags whose `cmd.Flags().Changed()` is true mutate their property, so `--editor-size large` alone won't wipe the label. Missing target block errors with `not found; use 'datatype block add'` (deliberate difference from `add`). Idempotent via `reflect.DeepEqual` — no PUT when the resulting block is byte-identical to the current one. `--label ""` / `--thumbnail ""` / `--settings-element-type ""` clear those optional fields. `editorUiAlias` and every other top-level value / sibling block survive the round-trip
- added GUID format validation to `--content-element-type` and `--settings-element-type` flags on `datatype block add` / `update` / `remove`. A typo'd GUID now errors with `must be a GUID (8-4-4-4-12 hex), got "..."` before any HTTP call instead of falling through to `block not found` (misleading) or silently persisting garbage on the server

## v0.3.16 - 2026-06-05

### Document fixes

- fixed `document update-properties --json` silently no-op'ing when given an object payload — keys landed at the document root instead of merging into `values[]`. The parser now accepts three shapes (object `{alias: value}`, array `[{alias, value, culture?, segment?}]`, envelope `{"values": [...]}`) and rejects malformed payloads loudly
- fixed `document update --save-and-publish` returning `{"published": null, "updated": null}` on 204 No Content — both flags are now `true` booleans on success
- added retry-on-race for the spurious `"culture for an [invariant content]"` 400 that the Management API throws under rapid back-to-back save-and-publish loops (200ms / 500ms / 1s backoffs, max 4 attempts). Other 400s surface immediately
- fixed `mergeAliasObjectArrays` collapsing culture-variant entries — values[] now keys by `(alias, culture, segment)` so a patch updating the same alias on two cultures doesn't overwrite one with the other

### New command surfaces

- added `member` command group (list / search / get / create / update / update-properties / delete / set-groups) and `member-group` (list / get), closing the gap where the entire backoffice Members section was unreachable from the CLI
- added `member` read-only-field guards: `member create` and `member update` reject patches containing `isApproved`, `isLockedOut`, `failedPasswordAttempts`, or `isTwoFactorEnabled` because the Management API silently ignores them (verified live against v17.4.2). Help text documents the API limitation explicitly
- added `document references <id>` / `document referenced-descendants <id>` / `document are-referenced --ids …` wrapping the Management API's tracked-references endpoints — answers "what uses this node" for orphan checks, safe-delete checks, and taxonomy usage audits
- added `media references <id>` / `media referenced-descendants <id>` / `media are-referenced --ids …` — symmetric port of the above to media assets

### Pagination

- added `--skip` and `--take` flags to `document/media/doctype children` and `root` — was capped at the server's default page (~100) with no way to walk past it. `--first-n` is a client-side cap on a single response; `--skip` lets you paginate
- added `--all` flag to the same commands for auto-paginated walks. Defaults to 500-item pages, capped internally at 100k items, honours `--first-n` as an early stop. Errors with a precise resume offset if it hits the safety ceiling without exhausting the collection

### Generated SKILL.md improvements

- the skill generator now propagates each command's `cobra.Long` help text into the generated SKILL.md, so agents reading the skill file see the same warnings a human gets from `--help` (notably the API-limitation callouts on `member create` and `member update`)

## v0.3.15 - 2026-06-04

- added `models-builder` command group (`dashboard`, `status`, `build`) wrapping `/umbraco/management/api/v1/models-builder/*`; `build --wait` polls until `Current` so scripts can sequence `doctype create → build --wait → dotnet build`. `build` pre-checks dashboard mode (refuses `InMemory`/`Nothing` with a clear message) and `canGenerate` (surfaces `lastError` instead of POSTing into a guaranteed failure). `--dry-run` returns the planned POST without triggering generation.
- added `datatype block add|remove|list` for Umbraco.BlockList and Umbraco.BlockGrid datatypes: read-modify-write helpers that mutate the `blocks` value entry without clobbering unrelated configuration. Idempotent on `contentElementTypeKey`. Pre-checks `editorAlias` so non-block datatypes are rejected before any PUT. BlockGrid placement flags `--allow-at-root` and `--allow-in-areas` default to `true` so registered blocks are placeable straight away (the server defaults both to `false` when omitted, which would silently register an invisible block). `--group` over BlockGrid's `blockGroups` array is a deferred follow-up.
- fixed `datatype update --json` silently dropping fields the caller didn't mention (`editorUiAlias`, `items`, `multiple`, etc.). Both `--json` and `--merge-json` now route through fetch-and-merge using the existing `mergeAliasPayload` helper, so a one-field `--json '{"description":"x"}'` no longer nukes everything else server-side.
- expanded `doctype.create --print-template` to advertise `isElement`, `allowedTemplates`, `defaultTemplate`, `historyCleanup`, `collection`, and a clearer `compositions` annotation. Added `--element` convenience flag on `doctype create` that forces `isElement=true`.
- added schema entries for `models-builder.dashboard`, `models-builder.status`, `models-builder.build` so `umbraco schema models-builder.*` resolves (matches the pattern used by `server.*`, `logs.*`, etc.).

## v0.3.14 - 2026-06-03

- fixed `forms record <formId> <recordId>` 404 on Umbraco v17.x — the Forms Management API does not expose a GET endpoint on `/form/{formId}/record/{recordId}` (only PUT), so the CLI now fetches the records list and filters client-side
- `forms record` matches against either the record's `uniqueId` (GUID) or its numeric `id`; help text documents both
- added `--scan` flag (default 500) controlling how many records are pulled for the lookup; rejects non-positive values
- not-found errors now distinguish "scan window exhausted (the record may exist outside the window)" from "definitively not present (API returned fewer rows than --scan)" so agents know whether to widen or stop

## v0.3.13 - 2026-06-02

- added `forms` command group wrapping the Umbraco Forms Management API (`/umbraco/forms/management/api/v1`) for read-only access: `forms list`, `forms children <folderId>`, `forms get <id>`, `forms records <formId>`, `forms record <formId> <recordId>`, `forms record-workflow-log <formId> <recordId>`
- added `RequestOptions.APIPrefix` override on the HTTP client so command surfaces can target Management API mounts other than the core `/umbraco/management/api/v1` without affecting existing commands
- `forms records` defaults to `--take 100` to prevent agents pulling thousands of submissions in one shot; pass `--take 0` for no limit, or override via `--params`
- `forms records` accepts `--state`, `--from`, `--to`, `--skip`, `--take` as pass-through filters, with `--params` taking precedence on key collisions

## v0.3.11 - 2026-05-12

- fixed `logs templates` on Umbraco v17 by routing it through `/log-viewer/message-template`, with 404-only fallback to the legacy templates route
- added `--skip`, `--take`, `--from`, and `--to` to `logs templates` for the endpoint's paging and date-range query parameters

## v0.3.10 - 2026-05-11

- fixed `logs list` and `logs search` for Umbraco v17 by routing them through `/log-viewer/log`, with 404-only fallback to legacy log viewer routes
- fixed log level filtering to send repeated `logLevel` query parameters, matching the v17 Management API controller binding
- hid the removed `logs levels` endpoint from generated CLI docs and return a clear unsupported-v17 message for old scripts
- improved `logs level-count` errors for `CancelledByLogsSizeValidation` with a narrower-window hint

## v0.3.9 - 2026-05-08

- fixed `doctype list` on current Umbraco versions by routing it through `/tree/document-type/root` before falling back to legacy document type endpoints

## v0.3.8 - 2026-05-08

- fixed `--fields` so it is now pure client-side projection and is no longer sent as `?fields=...`, avoiding endpoints such as `datatype list` that reject the query parameter
- added `--summarize`, `--ids-only`, and `--first-n` to `doctype list`

## v0.3.7 - 2026-05-08

- fixed `--fields` so it actually trims the response. The Management API ignores the `?fields=` query parameter, so `--fields` was wired through to the URL but had no observable effect. The CLI now performs the projection client-side: each item (or lone object) is trimmed to the comma-separated keys named by `--fields` before `--summarize` / `--ids-only` / `--first-n` run

## v0.3.6 - 2026-05-08

- fixed `media root` and `media children` so they hit `/tree/media/root` and `/tree/media/children?parentId=...` (the v17 Management API tree paths) before falling back to the legacy `/media/root` / `/media/{id}/children` routes that 404 on current Umbraco versions
- added `--fields` to `doctype root`, `doctype children`, `datatype root`, and `dictionary list` so the response-trimming flag is uniform across every collection-returning command, matching what `media` and `document` already accepted
- updated the `media.root` and `media.children` schema entries to advertise the tree paths as primary

## v0.3.5 - 2026-05-08

- fixed `media upload --type <alias>` so it resolves canonical Umbraco aliases (e.g., `umbracoMediaVectorGraphics`). The lightweight item/tree endpoints do not include an alias field, so the resolver now collects candidate IDs from search and tree-root and fetches `/media-type/{id}` for each to inspect the alias on the full model
- fixed `media upload` payload shape: every body now uses the `variants[]` envelope expected by the Management API. `culture` is JSON null for invariant media types and the supplied/default code for culture-varying types. Top-level `name` is never sent. Passing `--culture` against a media type that does not vary now warns and forces null (the API only accepts null in that case)
- extended `--summarize` / `--ids-only` / `--first-n` to `media root`, `media children`, `document root`, `document children`, `dictionary list`, and `datatype list` so the triage flags work uniformly across collection-returning commands

## v0.3.4 - 2026-05-08

- maintenance release with no user-visible changes

## v0.3.3 - 2026-05-08

- fixed `media upload --type SVG` and other friendly short names by translating them to canonical Umbraco aliases (`umbracoMediaVectorGraphics`, `Image`, `File`, `Folder`, `umbracoMediaAudio`, `umbracoMediaVideo`, `umbracoMediaArticle`) before lookup
- fixed `media upload --type <alias>` resolution by paginating `/media-type` with a 500-item window and falling through to the search endpoints, so canonical aliases like `umbracoMediaVectorGraphics` reliably match against the full media type catalog
- fixed `media upload --culture <code>` so it forces the variant payload shape (variants[] + culture-tagged values) even when the resolved media type is not detected as varying by culture; emits a warning so an `ContentTypeCultureVarianceMismatch` from the server can be traced back to the override

## v0.3.2 - 2026-05-08

- fixed `media upload --type SVG` and custom media type names by resolving media types from the live media type list by alias/name before create
- fixed `media upload` for culture-varying media types by emitting `variants` and culture-scoped values; added `--culture`
- fixed `datatype search --editor-alias` so filtering happens across internally paginated results before applying user `--skip`/`--take`

## v0.3.1 - 2026-05-08

- fixed `media upload --type <alias>` so it resolves media type aliases/names to IDs before creating media
- fixed `datatype search --editor-alias` so it performs deterministic CLI-side filtering and works without a separate search query

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
