# Changelog

## Unreleased

- README command counts are now asserted by a test against the registered commands; drifted per-group counts corrected; totals now count runnable leaf commands
- five commands over endpoints the CLI already had bindings for but no surface: `user-data delete <key> --force` (DELETE /user-data/{id}, the gap the v0.4.22 notes recorded), and the `blueprint` extras `audit-log <id>` (paginated, `--params` for orderDirection/sinceDate), `items --ids a,b` (GET /item/document-blueprint with repeated `id` values, one call instead of one per GUID), `ancestors <id>` and `siblings <id>`. `ancestors` and `siblings` are plain reads rather than paginated collections because their routes do not use the `{items, total}` envelope: ancestors answers with a bare array and siblings with `{totalBefore, totalAfter, items}`, windowed with `--before`/`--after` around the target instead of skip/take (plus `--folders-only`). Verified live on 18.1 against disposable entities: `audit-log` returned the blueprint's Save entry, `items` resolved a GUID to its name and document type, `ancestors` returned the folder chain after a `move` (and the chain ends with the entry itself, not its parent), `siblings --before 1 --after 1 --folders-only` returned the window with `totalBefore: 1` for the sibling left outside it, and `user-data delete` was gated without `--force`, rehearsed with `--dry-run` and then removed a disposable entry that the follow-up `get` 404ed on. Also checked the variant path end to end, which had only ever been exercised on invariant content: a culture-varying document type with a culture-varying textstring, a document with en-US and de-DE variants, `blueprint create-from-document`, `blueprint get`, `blueprint scaffold` and `document create --from-blueprint` all keep `culture` on every value and `culture`/`name` on every variant, and the created document read back both cultures unchanged — no normalisation fix was needed. Two findings recorded rather than changed here: `POST /document-blueprint/from-document` applies `--name` to the default-culture variant only, leaving the other variants named after the source document; and `--json` on `document create --from-blueprint` merges `values` entry-wise by alias+culture+segment but *replaces* the `variants` array wholesale (variant objects carry no `alias`, so the alias-keyed merge does not apply), so renaming one culture drops the others — that merge is shared with `document update --merge-json` and is worth its own change

## v0.4.22 - 2026-09-22

- `searcher` and `relation` command groups. `searcher list` and `searcher query <searcher-name> --term <text>` (with `--query` as an alias, plus `--skip`/`--take`/`--all`/`--fields`) read Examine's search side, completing the `indexer` group's index side: "is the index healthy" now has a companion "what does the index actually return". `relation list --type <id>` lists the relations recorded for one relation type, and `relation type list|get|items` read the relation-type catalogue; the Management API exposes relations per relation type only (no by-parent or by-child route), and the group help says so instead of implying a lookup that does not exist. Live on 18.1: `searcher query <index-name> --term home` returned 216 hits (5 read back with `--fields id,score`), `relation type list` returned the instance's relation types, `relation type get`/`items` resolved them, and `relation list --type <id> --take 5` returned 5 of 121 rows. Two API surprises are documented in the command help: `GET /searcher` can return an empty list on an instance whose indexes are registered without standalone searchers, and the name `GET /searcher/{searcherName}/query` accepts is often the *index* name (`ExternalIndex`) rather than the `searcherName` the indexer reports (`ExternalSearcher`), which 404s with "Could not find a valid searcher" — so the help points at `indexer list --fields name,searcherName` and says to try both
- added five small read/write command surfaces over endpoints the CLI did not cover: `tag list` (GET /tag, with `--query`/`--group`/`--culture`, pagination and `--all`), `media resize-urls --ids a,b --width --height --mode` (GET /imaging/resize/urls, repeated `id` values in one call), `doctype property-is-used <id-or-alias> --alias <propertyAlias>` (GET /property-type/is-used, the check to run before removing a property), and the `user-data` group (list/get/create/update over the authenticated account's key/value store, both writes with `--dry-run`). `language cultures` (GET /culture) was already present. Verified against a local Umbraco 18.1: `tag list` paged 72 tags and the filters narrowed as expected; `language cultures` returned 364 entries; `resize-urls` returned per-item `urlInfos` carrying the `rmode`/`width`/`height` query string for two of three ids (the third has no image file); `property-is-used` answered false for element-type properties and true for document-type ones; a disposable `zz-probe` user-data entry was created, read, updated and then removed again. Two API notes: the user-data list/get responses carry a `key` the OpenAPI model does not declare, and `DELETE /user-data/{id}` exists in 18.1 even though the CLI group does not expose it yet
- corrected the authentication-required message added in v0.4.21: it asserted that Umbraco Deploy's API on Umbraco Cloud does not accept the Management API bearer token. That was a misreading of a transient environment hiccup — the same environment accepts the token when healthy, and Deploy's API works on all environments. The message now describes only what was observed (a redirect to, or a page of, the backoffice login) and asks for a retry and a profile check before assuming a permission problem. The v0.4.21 changelog context saying otherwise is withdrawn
- `blueprint` — a command group for document blueprints, the reusable content presets an editor picks when creating a document, which had no CLI surface at all: `list`/`children` walk the blueprint tree, `get`/`scaffold` read a blueprint and the document skeleton it produces, `create --json` (with `--print-template`) builds one from scratch, `create-from-document <id> --name <n> [--parent <folder>]` captures an existing document as a preset, `update` takes the usual `--json`/`--merge-json`/`--backup` contract, and `move`, `create-folder`, `delete`, `delete-folder` organize and remove them. `document create --from-blueprint <id>` is the consuming side: it fetches the blueprint's scaffold, reduces it to a document create payload (documentType to an `{id}` reference, response-only `flags`, `editorAlias` and variant state/date fields dropped, the required `template` key seeded as null) and deep-merges any `--json` on top, so `--json` becomes optional and only the values being overridden need naming; a new `--parent` flag fills the parent when the payload omits it, and a scaffold with no variant name is refused before the POST rather than by the server. Verified live on 18.1 end to end against disposable entities: folder create, create-from-document (dry-run and real), list/children/get/scaffold, move into the folder, `update --merge-json` and `update --backup` (value read back changed both times), `create --json` from scratch, `document create --from-blueprint` with and without `--json` (overridden value and preset value both read back on the created documents), the `--force` gate on `delete`, the server's "folder is not empty" refusal on `delete-folder`, and deletion of everything created
- added the file-based asset groups: `partial-view` (11 commands), `script` (9), `stylesheet` (9) and read-only `static-file` (3). These resources are keyed by their virtual file-system path rather than by a GUID, so each group offers `list`/`children` over the tree, `get <path>` (with `--out <file>` writing the content to disk verbatim), `create --path <folder> --name <file>` and `update <path>` taking `--content` or `--content-file`, `rename <path> --name <new>`, force-gated `delete`/`delete-folder`, and `create-folder`; `partial-view` also exposes the built-in `snippets`/`snippet <id>` catalogue. `get --out` and `create/update --content-file` round-trip a file byte for byte, so an agent never has to quote a Razor view or a stylesheet through the shell. Two path encodings are needed and both are verified against Umbraco 18.1: `GET`/`PUT`/`DELETE /<resource>/{path}` are catch-all routes that take the path with its separators intact, while `PUT /<resource>/{path}/rename` needs the path as a single doubly-escaped route segment because Kestrel decodes percent-escapes once before routing. Relative path segments are rejected outright. Verified live on a local 18.1 instance for each writable group: create folder, create file (including a name with spaces), get, `get --out` (diffed byte for byte against the source), `update --content-file` with a read-back, `rename` with a read-back, `children`/`list`, then delete file and folder with the tree re-read empty; `static-file list`/`children`/`get` were verified read-only against existing files

## v0.4.21 - 2026-09-22

- fixed the redirect loop against Umbraco Cloud when an endpoint sends the caller to the backoffice login (agent-reported on 0.4.20: `deploy queue list --profile dev` died with `stopped after 10 redirects` as `returnPath` nested; the earlier `cannot unmarshal string` on the same profile was the login HTML being parsed as JSON). The API client no longer follows redirects: a redirect to `/umbraco?returnPath=…` (or a 200 that is the login page) is reported as `authentication required: GET <path> redirected (302) to … — the environment did not accept the bearer token for this endpoint` and exits 3, after exactly one request. Other redirects surface as their 3xx status; non-login HTML still passes through as text for `api`. Context from the report, not a CLI defect: on Umbraco Cloud, Deploy's own API under `/umbraco/deploy/…` does not accept the Management API bearer token even though the core API does, so `deploy transfer` against a Cloud environment stays blocked until it does — the CLI now says so instead of looping

## v0.4.20 - 2026-09-21

- fixed `deploy transfer --dry-run` failing against an Umbraco Cloud environment with `json: cannot unmarshal string into Go value of type map[string]interface {}` while working locally (agent-reported on 0.4.19). Deploy's `GET /configuration/client` came back as a JSON string holding the object rather than the object itself; every fetch-an-object path now unwraps a double-encoded body, and a body of the wrong shape is reported as "GET /configuration/client returned an array, not a JSON object: …" naming the request instead of a bare decoder error. Not reproducible locally (the local instance returns the object), so the fix is verified against a mocked double-encoded response; the informative error would have made the original report actionable in one step
- `api` sends any absolute path under another `/umbraco/…` mount (Deploy, Forms, Automate management APIs) relative to the host root automatically, so `api GET /umbraco/deploy/management/api/v1/configuration/client` works without `--raw-path` (agent-reported: the path got the core `/umbraco/management/api/v1` prefix and 404ed, and `--raw-path` was not discovered). Full core Management API paths are still normalized and relative paths still get the prefix; `--raw-path` remains for paths outside `/umbraco/` (e.g. `/media/…`)

## v0.4.19 - 2026-09-21

- `deploy transfer` — Umbraco Deploy's content transfer ("Transfer now" / "Transfer queue") from the CLI (agent-requested: data types that pin a start node store a content GUID; schema travels in `.uda`, the content it points at does not, and the backoffice was the only way to move it with GUIDs intact). `--node <id>` (repeatable, `--type document|media|member|dictionary-item|form`, `--descendants`, `--culture`) transfers those items to the environment Deploy is configured to target; `--queue` transfers the accumulated queue. Deploy resolves and includes dependencies at transfer time (`--ignore-dependencies` only where the environment allows it). `--dry-run` resolves the target, each item's name and descendant count, and shows the exact request without sending it; dependencies are computed server-side and cannot be previewed. Without `--dry-run` the command requires `--force` (it writes to another environment). By default it waits on the transfer session with progress on stderr: exit 0 on Completed, 5 on Failed/Cancelled/Mismatch (with the server's log and exception), 6 when `--timeout` elapses (status unknown; the transfer keeps running); `--wait=false` returns the session id. `deploy queue list|add|remove|clear` manage the server-side queue shared with the backoffice. The routes come from Deploy's own management API (`/umbraco/deploy/management/api/v1`, read from the Deploy 18 backoffice bundle since it publishes no OpenAPI document); an environment without the package gets a plain "not available" instead of a 404 hint. Verified live on 18.1 with Deploy 18.0.1: dry-runs (single node, 1170-descendant subtree, queue mode), queue add/list/remove/clear, unknown-id and ignore-dependencies guards. The transfer itself was not executed live — the local instance's target is a shared cloud environment — and is covered by mocked tests of the full session lifecycle

## v0.4.18 - 2026-09-21

- fixed `logs tail` printing nothing, forever, on a busy environment (agent-reported on 0.4.17; reproduced on 18.1). The log-viewer's `startDate`/`endDate` select which daily log *files* are read and do not filter entries by timestamp (a `startDate` an hour in the future still returns today's entries), so tail's ascending page was always the day's oldest 500 entries; every one was older than the cursor, and a full page triggered an immediate re-poll of the same page: zero output and a tight request loop. Tail now polls newest-first, keeps entries at or after its cursor client-side, pages back with `skip` through bursts (up to 20 pages per poll; a backlog larger than that stops the run with an error naming `logs search --from` rather than skipping entries), and prints oldest-first exactly once. A startup line on stderr states the cursor and interval, and `--heartbeat <duration>` adds still-alive lines naming the newest entry the server has, so a quiet environment is distinguishable from a blind tail. `--json` is accepted as on `deploy watch` (same as `-o json`). Live: the 0.4.17 binary printed 0 lines in 14 s against the local instance; the fix printed the 4 new entries once, in order

## v0.4.17 - 2026-09-15

- `doctype|mediatype|membertype remove-property <id-or-alias> --alias <propertyAlias>` removes one property and writes the type back otherwise unchanged (agent-requested: the alternative was get → hand-edit → `update --json`, which risks dropping fields such as `allowedAsRoot`). The type is re-read afterwards and the command fails if the property survived. Like every destructive command it refuses to run without `--dry-run` (plan) or `--force` (confirm); `--backup` saves the pre-change type first. Tabs/groups left without properties are pruned by the server on save — the result lists them under `prunedContainers` with a warning. An alias that differs only in case gets a "did you mean" hint instead of a write. Live on 18.1: removed two properties from a disposable type (second removal reported the pruned group), `allowedAsRoot` and the rest untouched
- `document publish --ids a,b,c | --from-file ids.txt` and `document update --ids … | --from-file …` run the same operation over many documents in one invocation (agent-reported: 124 pages took 372 CLI calls from a script). Sequential, one result row per document (`id`, `name`, `update`, `publish`, `backup`, `error`) plus counters; `--merge-json` and `--property` merge into each document's own current state (unchanged documents are `skipped`), `--save-and-publish` publishes each after its update (atomic `update-and-publish` on 18.1+, two calls on older servers), `--backup` saves each document first (`--backup=<dir>` puts the files there). A failing document does not stop the rest; the command exits non-zero when any row failed with the documented code of the failure (3 auth, 4 API, 1 local). `publish --ids … --json` sends the same full publish payload (e.g. several `publishSchedules`) to every document. Multi-document runs require `--force` or `--dry-run`; a dry-run shows the exact requests for the first three documents and counts the rest, so rehearsing 124 pages does not cost 124 reads. `document bulk-update` also accepts `--ids`. The `document` group help opens with a task → command map (bulk change, publish many, undo a write, CSV rows). Live on 18.1: 4 disposable pages plus a bogus id → 4 published, 1 failed, exit 4; update+publish with backups → all `Published` with the new value; unchanged rerun → 4 skipped, no writes

- `document update --backup[=path]` and `document update-properties --backup[=path]` save the current document before the PUT (agent-requested parity with `doctype update`/`datatype update`; content writes are the riskier ones). The backup is read from the server right before writing, never derived from the merged body, and its path is always in the result (also with `--save-and-publish`). `document restore-backup <file>` PUTs the saved document back and re-reads it, failing if any saved value alias is missing afterwards; publish state is not part of the backup. The same `restore-backup` now exists on `doctype`, `datatype`, `mediatype` and `membertype` (the envelope names its resource, so a document backup cannot be restored onto a type and vice versa; media/member types strip their response-only fields as `update` does). Live on 18.1: value changed → backup written → restore → original value back and verified; `--save-and-publish --backup` reports both results and the backup path

- fixed `doctype|mediatype|membertype get <alias>` failing for types in nested folders (agent-reported: `sharedSector`, `sharedAuthor`, `sharedSectors` under Shared › Sectors/Authors, while a one-level-deep alias worked). Reproduced live on a 208-type tree: the alias's first word (`shared`) matched no names, and the tree-walk fallback sent every type id in one batch URL, which the server never answered, so the alias was reported as unknown. The name search now also tries the alias's trailing word (`sector`) and the whole alias, and batch lookups are capped at 100 ids per request (the same chunking the list enrichment already used). Live: all three aliases resolve in about a second; an unknown alias walks the full tree in one second instead of hanging
- fixed `forms children <folderId>` returning every form on the install (agent-reported indirectly: a folder read like a form). `GET /form?folderId=…` ignores the filter on Forms 17/18 (verified live: the same 151 forms for any folder id, including a nonexistent one); the command now reads `GET /tree/form/children/{folderId}`, which returns the folder's forms and sub-folders. Because that tree route answers an empty page for any id, a non-folder id (a form id, a typo) is reported as such instead of as an empty folder
- `forms list` and `forms children` items now always carry `isFolder` and `type` (`"folder"` or `"form"`); form rows previously carried no flag at all, so a folder named like a form was indistinguishable from one. `forms get <folderId>` says "is a Forms folder, not a form; use forms children" instead of a bare 404 (an unknown id keeps the real 404, exit 4)
- `doctype|datatype|mediatype|membertype create-folder --name <n> [--parent <id>]` creates a folder in the type tree (POST `/<resource>/folder`; the folder is read back so the result is the persisted record), and `delete-folder <id>` removes an empty one (gated by `--force`/`--dry-run`; the server refuses non-empty folders with "The folder is not empty"). Agent-requested: folders could only be created through `api`
- `doctype create --print-template` now advertises `parent` (folder placement), `allowedDocumentTypes`, `allowedInLibrary`, and the property-level `description`, `validation`, `appearance`, `variesByCulture`/`variesBySegment` fields, all accepted before but absent from the skeleton (agent-reported). The version-cleanup block is now named `cleanup` as the Management API calls it — the skeleton said `historyCleanup` (Deploy's name), which the server ignored; a payload still using `historyCleanup` is renamed on the way out. `datatype create --print-template` gains `parent` likewise

## v0.4.16 - 2026-09-11

- fixed `doctype|mediatype|membertype list --types-only` (with or without `--recursive`) returning nothing (agent-reported): the folder heuristic treated any item without an alias as a folder, and tree items never carry one. An explicit `isFolder` flag is now authoritative in both directions
- `doctype|mediatype|membertype list`, `children` and `search` items now carry `alias` (and `isElement`), fetched through the batch route in one extra request per page — the Management API tree/search models omit them (agent-reported: `--summarize` promised alias and `--fields alias` dropped it). Data types have no alias; `--summarize` help now says so (they carry `editorAlias`)
- `api --dry-run` previews list the complete header set the request will carry: implicit `Authorization: Bearer ***`, `User-Agent`, and `Content-Type` (`application/json` for `--body`, `multipart/form-data; boundary=<generated when sent>` for `--form`), with `--header` values overriding
- global `--base-url <url>` overrides the resolved base URL while keeping the profile's credentials (agent-reported: no way to exercise failure paths without copying a secret into another config file)

## v0.4.15 - 2026-09-10

- `doctype get`, `mediatype get`, `membertype get` accept an alias as well as a GUID (agent-reported): a non-GUID argument is resolved through the item search (which matches names, so the first camelCase word of the alias is used as the query) and an exact, case-insensitive alias check on the candidates' full models (batch route when available). An unknown alias now says "not a GUID and no document type has that alias; use get <guid> or search --query …" instead of a 404 whose hint blamed the Umbraco version
- `api --dry-run` previews now include the request headers (`--header`), for JSON and multipart requests alike (agent-reported: a header could not be checked before sending)
- discoverability (agent-reported "no block command exists" against a build that had `datatype block reorder`/`--group`): the `datatype` and `doctype` groups open with task → command maps, and the doctype map states that allowed blocks, their order and Block Grid groups live on the Block List/Grid data type (`datatype block …`)

## v0.4.14 - 2026-09-09

- `datatype block reorder <id> --keys a,b,c` reorders the allowed blocks (array order is the picker order): listed keys first, unlisted blocks keep their relative order; idempotent, and the datatype is re-read afterwards so the command fails if the server persisted a different order. `datatype block add|update … --group <name>` places a Block Grid block in a block group, creating the group in `blockGroups` when it does not exist (names matched case-insensitively); `--group ""` on update removes the block from its group. `datatype block groups <id>` lists the groups with block counts. `--group` and `groups` are Block Grid concepts and are rejected on Block List (also when the block already exists, before the idempotent no-op); `reorder` works on both editors. Whitespace-only group names are rejected. Closes the two deferred items from the block round (agent-reported)

- `deploy apply` — Umbraco Deploy's "Update schema" from the CLI, the write side of `deploy status` (agent-requested). Compares every `.uda` artifact exactly as `status` does, then creates missing entities (keeping the artifact GUID so references resolve) and full-replaces drifted ones, in dependency order: Deploy's `Ordering` dependencies first, then kind precedence (languages → folders → data types → templates → member/media/document types → member groups). Forward references between content types (allowed children, compositions to a type created later in the same plan) are deferred and restored in a fix-up pass. Each write is re-read and compared again, so `applied` means the environment matches the artifact; `applied-drifted` and `failed` are reported with the remaining diffs or error, and a failure stops the run (`--continue-on-error` to keep going) with exit 4. Every updated entity is backed up first (`--backup-dir`, default `./.umbraco-deploy-backup/<timestamp>`). Mutating: refuses to run without `--dry-run` (plan only; `--bodies` includes the exact request bodies) or `--force`. Supported kinds: language, data-type and folders, document/media/member types and folders, template, member-group; relation types and Automate artifacts are read-only in the Management API and are listed as unsupported. Verified live on 18.1 with a six-artifact corpus: create pass (folders, data type, template, element type with grouped mandatory property, page type with allowed child, default template and cleanup) → all in-sync; drift pass (config value, template content, description + mandatory, allowed children) → four updates with backups → all in-sync
- `deploy apply` — Umbraco Deploy's "Update schema" from the CLI, the write side of `deploy status` (agent-requested). Compares every `.uda` artifact exactly as `status` does, then creates missing entities (keeping the artifact GUID so references resolve) and full-replaces drifted ones, in dependency order: Deploy's `Ordering` dependencies first, then kind precedence (languages → folders → data types → templates → member/media/document types → member groups). Forward references between content types (allowed children, compositions to a type created later in the same plan) are deferred and restored in a fix-up pass. Each write is re-read and compared again, so `applied` means the environment matches the artifact; `applied-drifted` and `failed` are reported with the remaining diffs or error, and a failure stops the run (`--continue-on-error` to keep going) with exit 4. Every updated entity is backed up first (`--backup-dir`, default `./.umbraco-deploy-backup/<timestamp>`). Mutating: refuses to run without `--dry-run` (plan only; `--bodies` includes the exact request bodies) or `--force`. Supported kinds: language, data-type and folders, document/media/member types and folders, template, member-group; relation types and Automate artifacts are read-only in the Management API and are listed as unsupported. Plan errors (unparseable, unmappable, or uncomparable artifacts) exit 4 — also under `--dry-run` — and block a forced run unless `--continue-on-error`; the content-type comparison (shared with `deploy status`) now also covers allowed/default templates, history cleanup, collection, variations and containers, so `apply` never declares in-sync a field it would write; folder parents are flagged as warnings because the Management API exposes neither a folder parent nor a folder move. API errors now lead with the server's ProblemDetails title (the exception message) instead of the stack trace that alphabetical JSON put first. Verified live on 18.1 with a six-artifact corpus: create pass (folders, data type, template, element type with grouped mandatory property, page type with allowed child, default template and cleanup) → all in-sync; drift pass (config value, template content, description + mandatory, allowed children) → four updates with backups → all in-sync

- fixed `doctype get` / `mediatype get` / `membertype get` reporting any unknown id as "is a folder, not a document type" (agent-reported): the folder probe asked the tree for children of the id, and the tree answers 200 with an empty page for *any* GUID. The probe now hits the folder endpoint itself, so a typo, a deleted type, or the wrong environment surfaces as the real 404 (exit 4) and only actual folders get the folder hint. The 404 hint now also tells a missing *entity* (server ProblemDetails, "the id does not exist in this environment") apart from a missing *route* ("may not be supported in your Umbraco version"), which previously read the same
- `media inspect <id>` (alias `info`) answers "which file is this?" in one read: name, media type, file src and public URL, extension, bytes, raster width/height from the derived values, SVG viewBox/width/height read from the file itself, edited crops and focal point, plus the remaining values. `media download <id> <path>` (alias `get-file`) saves the file verbatim, using the server-side name when given a directory; `--dry-run` reports the resolved destination (and whether it would overwrite) without fetching. Both take `--culture`/`--segment` on culture-varying media and refuse to guess between variants; `inspect` lists the other variants under `otherVariants` and fails (exit 4) when the URL lookup fails instead of quietly omitting it
- discoverability (agent-reported: `media references` existed but was not found when the task was phrased as "which content uses this media"): the `media` group help now opens with a task → command map, `media references` gains the aliases `find-references`, `referenced-by` and `usage`, `urls`/`url` and `replace-file`/`replace` likewise, and generated skills render group-level help and aliases so agents reading SKILL.md see the same map. New recipe skill `replace-media-file-safely`
- `api --out` now streams the response body to a `.part` file beside the destination and renames it on success, so a multi-hundred-MB asset no longer has to fit in memory and a failed download never leaves a truncated file; the summary reports the status the server actually returned (e.g. 206 for a `Range` request) instead of a constant 200
- `media replace-file <id> <file>` swaps the file behind an existing media item while preserving every other value (agent-reported gap; the fallback was a raw PUT that wiped the item). Reproduced live on 18.1: a PUT whose `temporaryFileId` does not resolve — expired, already consumed, or mistyped — returns 200 and leaves the item with zero values. The command therefore re-reads the item after the write and fails when it was emptied or the file property is gone, instead of reporting success. `--name` renames on the same request; `--property` targets a non-default file property; `--culture`/`--segment` select the variant on culture-varying media (required when several exist — the command refuses to guess). `restore-backup` derives its endpoint from the validated id (never from the file), confines re-uploaded binaries to the backup's `.files` folder, and auto-named backups carry a random suffix and are created exclusively so nothing is ever overwritten
- `--backup [path]` on every `update` command saves the pre-change entity to a JSON envelope before the PUT (bare `--backup` derives the name from collection, id and timestamp; `--backup=<path>` chooses it). On `media replace-file` the backup also downloads the current binary, because Umbraco deletes the replaced file — a metadata-only backup would restore a dead reference
- `media restore-backup <file>` PUTs a backup back, re-uploading the saved binary when present, and refuses a metadata-only restore whose file is no longer served. Verifies the item is not empty afterwards
- every outbound request (token, Management API, multipart, raw) now carries `User-Agent: umbraco-cli/<version> (<os>; <arch>)` instead of Go's default `Go-http-client/1.1`, which Cloudflare-fronted hosts flag as a bot — the cause of an agent-reported 403 on a live site that only a hand-set header got past
- `api` gains `--form field=value` / `--form field=@path` (multipart/form-data, e.g. `POST /temporary-file`), `--header "Key: Value"` (repeatable), and `--raw-path` (send the path relative to the host root — Automate, Forms, or a public `/media/...` asset — instead of the Management API mount), and `--out <file>` (GET only; writes the response body verbatim so binary assets survive — the structured output re-encodes bodies as JSON strings). Previously `api` was JSON-only and force-rooted every path under `/umbraco/management/api/v1`, so a file upload required leaving the CLI entirely. Header names are canonicalized so a later `--header` deterministically overrides an earlier spelling
- `deploy watch` probes (management liveness and public health paths) now carry the same User-Agent as every other request, so a Cloudflare-fronted host that blocks Go's default agent cannot leave the watch stuck before `serving`

## v0.4.13 - 2026-09-01

- fixed the `doctype add-container` → `add-property` workflow, which could never work (agent-reported, reproduced live on 18.1): the server prunes containers saved with no properties while still answering success, so `add-container` reported `updated: true` for a container that silently vanished, and `add-property` then refused to target it. `add-container` now verifies the container survived the save and errors honestly when it was pruned (with the working alternative in the message), and `add-property` gains `--create-container` (+ `--container-type Group|Tab`) to create the container together with its first property in one request — the only shape the server persists

## v0.4.12 - 2026-08-27

- `deploy status` with an explicit `-o json` now exits quietly on drift (still exit 7): the JSON report already carries the summary, and CI harnesses that merge stdout and stderr were corrupting the JSON with the redundant stderr summary line — the cause of two consecutive field reports of "invalid JSON output". Terminal runs (no `-o json`) keep the human summary line on stderr. Also re-verified against the released 0.4.11 binary, for the record: the drift exit code is the constant 7 (one drifted artifact → exit 7), never the drift count — the reported 7-drifted/exit-7 match was the same coincidence twice

## v0.4.11 - 2026-08-26

- fixed `deploy watch` verifying too early (HIGH, from the first production run): a single passing index check could land in the healthy gap between the app serving and the deployment pipeline discarding the replicated-clean indexes — observed live: `verified` exited 0 twenty-seven seconds before Umbraco Deploy wiped every Examine index, leaving search empty for 17 minutes while CI had gone green. `verified` now requires the environment to stay healthy (health paths + indexes) for a full `--settle` window (default 90s, measured from first all-clear; `0` restores single-sample) after a new `settling` phase; disturbances emit `settle-interrupted` with the reason and index names — visible, per the command's silence-is-never-ambiguous design — and restart the window on recovery, so `verified` means search actually works. The settle clock starts at all-clear rather than at `serving` deliberately: measured against the production timeline, a 60s-after-serving window would have missed that rebuild by 17 seconds
- fixed `deploy status` field findings from the same run: the command help still promised exit 2 for drift (stale — the actual code is the documented 7; the reported "exit code equals the drift count" was a coincidence of seven drifted artifacts), and now also documents that the drift summary line goes to stderr so `-o json` stdout stays parseable (the observed invalid JSON came from a `2>&1` capture); language artifacts (`umb://language/en-US` — languages are keyed by ISO code, not GUID) now parse and compare via `GET /language/{isoCode}` instead of erroring on every run, while GUID-keyed kinds still reject non-GUID identifiers before any request; and `--kind` filters now exclude out-of-scope errored artifacts from the summary

## v0.4.10 - 2026-08-21

- fixed `webhook update --merge-json`, which failed server validation on every call (agent-reported): the Management API returns `events` as objects (`{eventName, eventType, alias}`) but its update/create models require alias-string arrays, so the fetch-and-merge round trip PUT objects back and the server rejected them even when the update did not touch events. Both `webhook update` and `webhook create` now map object-form events down to their aliases (string entries pass through; an object without an alias is rejected with a pointer to `api GET /webhook/events`); verified live with a disposable webhook on both the merge path and pasted-`webhook get`-output
- added `deploy status` (part 3, completing the deployment-monitoring request): reads the site repo's Umbraco Deploy `.uda` artifacts (`--uda-dir`, BOM-tolerant, discriminated by Udi entity type) and compares each against the target environment read-only, reporting `in-sync | drifted (with the differing fields) | missing-remote | unknown | error` per artifact plus a summary — the pre-flight that turns "will this deploy carry surprises?" into an answerable question, since Deploy skips in-sync artifacts and processes drifted ones. Covers data types (artifact-carried configuration keys only, so environment-only migration markers are not drift), document/media/member types (both property collections — grouped and top-level), templates (line-ending-normalized content), containers, member groups, and relation types; Automate artifacts degrade to `unknown` where that API is unavailable (never a false in-sync) while step aliases are still read locally and `--flag-step-alias` marks automations carrying aliases your Deploy version cannot validate (configuration, never encoded). Exit 7 on drift/missing (its own documented code — 2 stays reserved for `schema diff`; suppress with `--exit-zero`)
- added `deploy watch`, an effect-based deployment monitor (part 2 of the deployment-monitoring request): observes the target environment for state deltas only a deployment can cause — no pipeline or portal API, so it works identically on Umbraco Cloud and on-prem, strictly read-only. Signals: the newest log entry's ProcessId/MachineName (an app recycle = the deploy landed), the token endpoint probed unauthenticated (503/unreachable = down, 401 = alive — the earliest all-clear), configurable public `--health-path`s, and Examine index health ("deploy succeeded" and "the site works" are different questions — a deploy-triggered rebuild leaves search empty until it finishes). Emits timestamped phase transitions `baseline → restarting → app-alive → serving → landed → verified | failed | timeout`; everything is baselined before arming (a signal already true on the target is not a signal, so pre-existing bad indexes/paths are excluded); success is never inferred from silence (`--heartbeat` keeps stderr alive, `--timeout` exits 6 "status unknown", `--escalation` exits 5 on sustained downtime or post-landing health failure); `--json` emits NDJSON for CI. Exit codes 5 and 6 join the documented contract
- added `logs errors` for post-incident/post-deploy triage: Error+Fatal entries in a window (`--since`/`--until`, default last 24h), and `--distinct` groups them into fingerprinted error classes (message template + normalized exception head, so two SQL violations with different constraint names are different classes) with count, first/last seen, levels, source contexts, and an example — sorted newest-first-seen so new breakage tops the list; known-chronic classes drop via `--suppress <fingerprint>` / `--suppress-contains <substring>` (per-site configuration, never encoded) and suppressed groups stay counted in the summary
- added `doctype reorder-properties <id>` closing an agent-reported gap: the Management API has no property-reorder operation (order is the per-container `sortOrder` field, changed only via a full document-type PUT), so reordering previously meant hand-rolling a read-modify-write through `--merge-json`. Two modes: `--aliases a,b,c` assigns positions with the container's remaining properties following in their current order, and `--alias x --sort-order n` moves one property; both verified live on 18.1

## v0.4.9 - 2026-08-12

- fixed `--culture` on `document publish` (and the new `element publish`): the shortcut sent `{"cultures":[...]}`, which belongs to the unpublish model — the publish operation requires `publishSchedules` on every Management API version the CLI has vendored, so culture-scoped publishes were rejected by the server; the shortcut now builds a `publishSchedules` entry (caught by Codex review)
- `are-referenced` bulk checks (document/media/element) now request as many rows as IDs supplied instead of relying on the server's default page size of 20, so IDs beyond the first page can no longer be misread as unreferenced
- added the `element` command group for the Umbraco 18.1+ element library (reusable content items living in a folder library instead of the page tree): `list`/`children`/`ancestors`/`search`, `get`/`published`, `create` (with atomic `--publish`), `update` (with atomic `--save-and-publish`), `publish`/`unpublish`, `copy`/`move`/`trash`/`delete`, `audit-log`, reference tracking (`references`/`referenced-descendants`/`are-referenced`), the `bin` recycle-bin subgroup with `restore`, and `version` history with rollback and prevent-cleanup — 21 commands mirroring the document family through the shared builders, all verified live against 18.1
- added `doctype allowed-in-library` listing the document types usable as library elements (`allowedInLibrary`), the discovery entry point for `element create`
- `user get` now accepts several IDs, fetching them in one round trip via the new `GET /user/batch` operation (Umbraco 18.1+); a single ID keeps using `GET /user/{id}`
- added `user set-language <iso-code>` wrapping the new `PUT /user/current/profile` operation (Umbraco 18.1+): sets the backoffice UI language of the account the CLI authenticates as
- added `document sort-children [parent-id]` and `media sort-children [parent-id]` for the new server-side reorder operations (Umbraco 18.1+): sorts every child of the parent (root when omitted) by `--field Name|CreateDate|UpdateDate` with `--direction` (asc/desc accepted, canonical casing sent), and `--culture` on documents for variant-name sorting — complementing the explicit-order `sort` commands (and `media sort` now exists: the document implementation generalized to one shared builder over `PUT /media/sort`, closing the gap where media had no manual reorder at all)
- added `document create --publish [--culture <csv>]` targeting the new atomic `POST /document/create-and-publish` operation (Umbraco 18.1+): the document is created and published in one server-side call; `--culture` names the cultures to publish, omitted = invariant (empty `culturesToPublish`, matching the backoffice convention)
- `document update --save-and-publish` now uses the atomic `PUT /document/{id}/update-and-publish` operation when the server supports it (Umbraco 18.1+), which eliminates the invariant-content publish race the two-call flow had to retry around; older servers fall back to the previous separate update+publish calls, and the output gains an `atomic: true` marker on the atomic path
- fixed `health run` and `health action` against current Umbraco versions (verified live on 18.1): both now call the modern operations first — `POST /health-check-group/{name}/check` and `POST /health-check/execute-action` (the positional health-check id fills `healthCheck.id` when `--json` omits it) — and fall back to the legacy `GET .../run` / `POST /health-check/{actionId}` routes on 404; previously both commands 404'd on current servers (reported by an agent against 18.1)

## v0.4.8 - 2026-07-03

- added `media restore` completing the media recycle-bin lifecycle: restores to the original parent by default (looked up via the recycle-bin API), `--to <parent-id>` overrides, `--to root` restores at the media root — the document restore logic generalized into one shared implementation for both resources
- added `--wait`/`--timeout`/`--poll-interval` to `published-cache rebuild`, polling the rebuild status until `isRebuilding` clears — parity with `indexer rebuild --wait`
- every command that declares no positional arguments now rejects stray ones (79 commands across all groups) — previously a stray ID was silently ignored, which is how a mistyped `bin delete <id>` could have become a full-bin `empty`; a permanent invariant test walks the whole command tree so new commands cannot regress
- added `logs tail` for following new log entries as they arrive: the Management API has no streaming endpoint, so tail polls the log-viewer with a moving cursor, deduplicates boundary entries, and prints each entry exactly once — NDJSON per line for json output, formatted lines otherwise — with `--level`/`--source-context`/`--path`/`--contains`/`--correlation-id` filters, `--redact`/`--redact-default`, `--since`, `--interval`, and `--for` (bounded runs for agents; exits 0 on interrupt or elapse)

## v0.4.7 - 2026-07-02

- removed the vendored extension-development skills bundle and its Node toolchain (a PoC-era aggregation of `umbraco/Umbraco-CMS-Backoffice-Skills` content): `skills/` now contains only the CLI-generated `skills/cli/`, the repo is pure Go with no Node.js requirement, and extension-development skills should be sourced from their upstream repo
- documented the exit-code contract and gave failure classes distinct codes: 0 success, 1 usage/local error, 2 `schema diff` differences found, 3 authentication/credential failure, 4 Management API error response — CI gates and scripts can now branch on `$?` instead of parsing stderr; shell completions (bash/zsh/fish via `umbraco completion`) documented in the README
- expanded `schema diff` beyond doctype/datatype: `--entity` now also accepts `mediatype`, `membertype`, `template` (nested template trees walked, `masterTemplate` references compared by alias), `language` (identified by ISO code), and `dictionary` (translations compared); cross-environment ID references for data types, document/media/member types, and templates are normalized to aliases so identical schema on both sides diffs clean; the default entity set stays doctype,datatype
- added `mediatype` and `membertype` command groups completing the schema type family alongside `doctype`: `list` (with the same `--recursive`/`--types-only` folder handling doctype has), folder-aware `get` errors, `children`, `search`, `create`/`update`/`delete`, and `export`; the doctype folder-tree helpers were generalized so all three resources share one implementation
- added recycle bin subgroups `document bin` and `media bin` completing the trash lifecycle: `list` (paginated bin root), `children <id>` (descend trashed subtrees), `original-parent <id>` (the default restore target), `delete <id>` (permanently delete one trashed item, gated), and `empty` (destroy everything in the bin, gated behind `--force`/`--dry-run`)
- added `indexer` command group for Examine search indexes: `list` (paginated, health status and document counts), `get <index-name>`, and `rebuild <index-name>` (gated behind `--force`/`--dry-run`, with `--wait`/`--timeout`/`--poll-interval` to block until the index leaves Rebuilding) — the standard fix path for missing or stale search results
- added `redirect` command group for the redirect URL tracker: `list` (paginated, `--filter` URL substring), `get <document-id>` (redirects recorded for one document), `delete` (gated behind `--force`/`--dry-run`), `status`, and `enable`/`disable` toggles; all commands schema-introspectable and verified live against a local instance
- added `published-cache` command group for stale-content incident response: `status` reads the rebuild state (falls back to the legacy status route on older versions), `rebuild` rebuilds the published cache from the database (gated behind `--force`/`--dry-run` — expensive on large sites), and `reload` refreshes the in-memory cache cheaply; verified live against a local instance
- consolidated the command layer: a new `createCommand` builder replaces seven near-identical create implementations (document, media, member, user, invite, doctype, datatype) so the create contract cannot drift per resource; destructive-command gating collapses into one `requireForceOrDryRun` helper with canonical wording; `member list`/`member search` now use the standard pagination flags and sentinel; oversized files split by concern (`logs.go` 850→152 lines plus query/output helpers, `document.go` gains dedicated publish and bulk files); command conventions documented in `internal/commands/CONVENTIONS.md`
- decomposed the 1,000-line config package internals for maintainability: profile management, source loaders, and user-config writing now live in dedicated files, .NET host-project discovery moved to a new `internal/dotnet` package, and the documented config precedence is expressed as an explicit ordered source list in code; behavior is unchanged (all existing tests pass unmodified) while config test coverage rose from 65% to 84% and discovery gained its own fixture suite (93%); CI coverage floor ratcheted 74.5% -> 76%
- added CI quality gates: golangci-lint (errcheck, staticcheck, govet, unused, ineffassign, misspell, unconvert) now runs on every push/PR with all existing findings fixed, and total test coverage is enforced against a floor (74.5%) that ratchets up as coverage grows; auth token provider coverage rose from 52% to 97% (caching, expiry margin, malformed responses) and the health command group from 41% to 86%
- hardened the API client: media/file uploads (`MultipartPost`) now retry rate limits (429) and refresh expired tokens (401) through the same shared retry path as JSON requests instead of failing immediately; computed retry backoff gained jitter so concurrent commands rate-limited together do not retry in lockstep (server-provided `Retry-After` is still honored verbatim); API error messages cap the rendered response payload at 500 bytes with a `…(truncated)` marker so pathological error bodies cannot flood terminals or agent context windows (the full payload remains available programmatically); and the HTTP transport now enforces TLS 1.2 as a minimum version

## v0.4.6 - 2026-06-30

- added `document urls <id> [<id>...]` for the Management API's batch document URL endpoint, with `--culture`, `--absolute`, clean `-o plain` URL output, message-preserving table output, non-zero exit when a requested document has no published URL, and `document get --with-urls`
- added Automate authoring guardrails: `automate catalogue operators` now lists condition/filter operator strings plus Deploy `.uda` integer mappings, automation export/template/help text warns that import/update payloads use lowercase `operator` string values, and `automation validate` now points existing automation edits to `import-update --dry-run`
- added `schema diff <envA> <envB>` for cross-environment schema checks: compares configured profiles, fetches document types and data types from both environments, normalizes volatile IDs/order, reports added/removed/changed schema entities, supports `--entity`, `--include`, `--exclude`, stable JSON output, and `--exit-zero` for automation
- added `document grep <substring>` for exhaustive CLI-side document content scans: walks the document tree, fetches draft and/or published document payloads, scans serialized property values for exact substrings or regexes, supports property/document-type/subtree filters, emits progress on stderr, and reports skipped document fetches without corrupting JSON output
- added token-efficient document read output: `document get`, `document root`, `document children`, and `document search` now support CLI-side dotted `--fields`, compact `--summary`, recursive `--no-empty`, and explicit `--full` output controls without changing API requests or pagination semantics

## v0.4.5 - 2026-06-24

- hardened `logs list`/`logs search` for incident workflows: `--from`/`--to` and `--around ... --minutes N` are now enforced client-side so out-of-window rows returned by the Management API are filtered before output. Added deterministic client-side `--source-context`, `--path`, `--contains`, and `--correlation-id` filters; `--flat` JSON with `properties` as an object; `--redact`/`--redact-default`; `--count-by level|source|path`; and explicit pagination metadata with `--cursor`/`nextCursor`

Fixes from agent workflow reports against multi-environment Management API usage:

- added first-class config selection: global `--profile <name>` loads `~/.umbraco/<name>.config.json`, global `--config <path>` loads an explicit config file, and `auth list` / `auth use <profile>` manage stored profiles without exposing client secrets
- added `umbraco api <METHOD> <PATH>` for authenticated raw core Management API calls, including repeated query parameters, `--body @payload.json`, dry-run output, and status-code/body reporting for endpoint investigation
- improved document type tree workflows: `doctype list --recursive --types-only` walks folders and returns real document types, `--exclude-folders` is an alias, and `doctype get <folderId>` now reports that the ID is a folder instead of surfacing the generic endpoint 404
- clarified `auth status`: `hasCredentials` reports credential presence, while `authenticated` now reflects successful verification so a failed token request cannot look authenticated

## v0.4.4 - 2026-06-15

Fixes from real-world agent field testing against a Cloud instance:

- fixed `logs list`/`logs search` silently ignoring `--level` and `--filter-expression` whenever a `--params` blob was also supplied: the two input sources were mutually exclusive, so `logs search --level Error --params '{"take":30}'` dropped the level and returned newest-N unfiltered. The Management API honors both filters (confirmed live); flags now layer on top of `--params` and win on key conflicts. High-severity for incident triage
- corrected `automate run get` help: it promised "per-step inputs, outputs, errors, timing", but the Automate API's run model exposes only status, error, retry count, and timing per step — resolved step values are never returned. The help now says what the response actually holds and names the API limit, so agents stop digging for data the server cannot provide
- `-o table` now renders list responses as real column tables instead of one JSON-encoded `items` blob: a header row with one column per field (identity columns `id`/`name`/`alias`/`status`/`state` first, the rest alphabetical), nested values as compact truncated JSON (full data stays in `-o json`), control characters sanitized so columns hold, and a `(N of M)` footer on partial pages. Detail responses keep key/value rows; `--first-n` triaged envelopes still render as tables

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
