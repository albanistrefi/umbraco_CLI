# Command conventions

Rules for adding or changing commands under `internal/commands`. The
spec-based builders in `cmdkit` encode most of them — reach for a builder
first; write a custom `RunE` only when the command's contract genuinely
differs, and say why in a comment.

## Layout

| Package | Holds |
|---|---|
| `internal/commands` (`commands`) | Core CMS resources (document, media, doctype, …), `RegisterAll`, and the tree-wide tests (command counts, README counts, schema coverage, skills) |
| `internal/commands/cmdkit` | The kit for building commands: `Dependencies`, the builders and their specs, flag registration and gates, `--params`/`--json` parsing, endpoint fallback and `--all` paging, response reading and output shaping, printing, the `--merge-json` merge, `--backup`. `doc.go` maps each file to its part |
| `internal/commands/cmdtest` | The shared test harness: fake transports (`RoundTripper`, `JSONResponse`, `NoContent`, `TokenOr404`), `Deps`/`ClientDeps`/`MakeDeps`, `BuildRoot`, `Execute`/`ExecuteWithErr`. Imported only by `_test.go` files |
| `internal/commands/forms`, `automate`, `deploy`, `engage` | One package per Umbraco add-on, each exposing `Register(root, deps)` |

Small helpers that are not about building commands live in packages named
for what they do, outside `internal/commands`:

| Package | Holds |
|---|---|
| `internal/uuid` | `Valid` (GUID shape check), `NewV4` |
| `internal/jsonvalue` | Values decoded from JSON into `any`: `String`, `Text` (integral numbers in plain decimal), `ShapeName` |
| `internal/safefile` | Server-provided names made safe on disk: `Name`, `ChildPath` |
| `internal/api` | `IsStatus(err, code)` sits with `APIError` |

Import rules, which keep the graph acyclic:

- `cmdkit` imports no command package; `cmdtest` imports only `cmdkit`.
- Add-on packages import `cmdkit` (and, in tests, `cmdtest`), never
  `commands`. One add-on may import another when it needs that add-on's API
  facts (`deploy` uses `automate.APIPrefix`).
- `commands` imports the add-ons only to register them in `RegisterAll`.
- Core-resource logic an add-on needs (e.g. indexer health, the log-viewer
  route) moves into `cmdkit` rather than being reached through `commands`.
- Every package, core included, calls the kit as `cmdkit.X` and the harness
  as `cmdtest.X`; there are no local aliases. Name new files for what they
  hold — no `common.go`, `util.go` or `values.go`.

### Adding an add-on

1. Create `internal/commands/<addon>/` with `package <addon>` and a
   `Register(root *cobra.Command, deps cmdkit.Dependencies)` that adds the
   group. Keep the add-on's API mount in an unexported `<addon>APIPrefix`
   const passed through each spec's `APIPrefix` (export it only if another
   package needs it).
2. Call `<addon>.Register(root, deps)` from `RegisterAll` in `register.go` —
   the one registration list the production root and the tree-wide tests
   share.
3. Test the group in-package against a root carrying only that group,
   `cmdtest.BuildRoot(t, deps, Register)`. A test that needs the full tree
   (the core `schema` or `generate-skills` commands) goes in an external
   `<addon>_test` package, which may import `commands`
   (`cmdtest.BuildRoot(t, cmdtest.MakeDeps(), commands.RegisterAll)`); see
   `automate/tree_test.go`.
4. Add the schema entries, README counts and `ExpectedCollectionCommandCounts`
   as for any group, and regenerate `skills/cli/`.

## Builders

| Shape | Builder | Contract |
|---|---|---|
| Get by ID | `GetCommand` | `--fields` + client-side projection |
| Paginated read (root/children/list) | `CollectionCommand` | `--fields`/`--params`/`--skip`/`--take`/`--all` + triage, endpoint fallback |
| Search | `SearchCommand` | `--query` + extras merge into `--params`; `--params` wins on collisions |
| Create | `CreateCommand` | required `--json`, optional `--print-template`, CLI-generated id, identity-echoing result |
| Update | `UpdateCommand` | exactly one of `--json` (replace) / `--merge-json` (fetch-and-merge) |
| Move/copy | `TargetActionCommand` | `--to` shortcut or raw `--json`, method/route fallback |
| Hard delete | `DeleteCommand` | gated by force/dry-run |
| References | `ReferencesCommand` / `AreReferencedCommand` | shared document/media reference reads |

All live in `cmdkit` (`cmdkit.GetCommand`, `cmdkit.CollectionCommand`, …).

## Flags

- `--ids`: one comma-separated GUID list. When a command takes **two** ID
  lists, name both explicitly (`--user-ids`, `--group-ids`) — never leave one
  ambiguous `--ids` next to a qualified one.
- `--query`: server-side search input. `--filter`: substring filter parameter
  on filter-style endpoints (`/filter/...`).
- `--params`: raw JSON query parameters. Convenience flags fill missing keys;
  `--params` wins on collisions (see `MergeParams`).
- Pagination: always `AddPaginationFlags` (`-1` sentinel = not sent) plus
  `AddAutoPaginationFlag` where auto-paging is supported. Never a `0` default
  or a `Flags().Changed` check.
- Output shaping: `--fields` everywhere; document-shaped payloads add
  `--summary`/`--no-empty`/`--full` via `DocumentOutputTrim`.
- Every mutation takes `--dry-run` (`AddDryRunFlag`).

## Destructive operations

Gate with `RequireForceOrDryRun(cmd, "<consequence>", force, dryRun)` — the
canonical message is `<command> <consequence>; pass --force to confirm or
--dry-run to rehearse`. Hard deletes and bulk mutations are gated;
reversible recycle-bin moves (trash) intentionally are not.

## Output

- Always end with `PrintResult` (or `PrintMutationResult` for mutations, so
  an empty 204 success prints `{"<verb>": true}` instead of `null`).
- JSON output is the stable machine contract: only additive changes. Table
  layout is unstable and may reorder.

## File size

Keep command files under ~600 lines. When a group grows, split by concern
the way logs (`logs_query.go`/`logs_output.go`), document
(`document_publish.go`/`document_bulk.go`/`document_lifecycle.go`) and
deploy (`deploy/status.go`/`status_compare.go`/`status_automate.go`) do —
registration and core CRUD stay in the group's main file.
