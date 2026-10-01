// Package cmdkit is the shared toolkit every command package builds on: the
// injected Dependencies, output printing, the spec-based command builders
// (get/collection/search/create/update/delete/target-action/references),
// flag helpers, payload parsing and merging, endpoint fallback and
// auto-pagination, --backup support, and small value/ID helpers.
//
// It must not import any command package. Core resources (package commands)
// and each add-on (forms, automate, deploy, engage) import it, which is what
// lets the add-ons live in their own packages without an import cycle.
package cmdkit
