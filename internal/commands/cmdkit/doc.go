// Package cmdkit is the kit every command package builds its commands from.
//
//   - deps.go: Dependencies, the client/config/output wiring each builder gets.
//   - archetypes_read.go, archetypes_write.go: the spec-based builders
//     (get/collection/search/references, create/update/target-action/delete).
//   - flags.go: shared flag registration and required-option/--force gates.
//   - params.go, payload.go: --params query parameters and --json bodies.
//   - fallback.go: endpoint fallback across Management API versions and
//     --all auto-pagination.
//   - response.go, projection.go, output.go: reading responses, shaping them
//     (--fields, triage, document trim) and printing them.
//   - merge.go: the --merge-json deep merge.
//   - backup.go: --backup envelopes and restore input.
//   - management.go: Management API facts shared by a core group and an add-on.
//   - logtail.go: the log-viewer tail behind 'logs tail' and 'deploy watch
//     --logs', and the log entry helpers (timestamps, message text, redaction).
//
// It must not import any command package. Core resources (package commands)
// and each add-on (forms, automate, deploy, engage) import it, which is what
// lets the add-ons live in their own packages without an import cycle.
package cmdkit
