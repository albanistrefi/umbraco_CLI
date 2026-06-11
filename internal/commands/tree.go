package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

type treeNodeRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func RegisterTree(root *cobra.Command, deps Dependencies) {
	tree := &cobra.Command{Use: "tree", Short: "Tree navigation helpers"}
	tree.AddCommand(treeWalk(deps))
	root.AddCommand(tree)
}

func treeWalk(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "walk <path>",
		Short: "Resolve a content tree path like Home/Partners/Partner List to a node ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			segments := parseTreePath(args[0])
			if len(segments) == 0 {
				return fmt.Errorf("tree walk requires a non-empty path")
			}

			ctx := cmd.Context()
			var current treeNodeRef
			var err error
			for index, segment := range segments {
				if index == 0 {
					current, err = findDocumentRootByName(ctx, deps, segment)
				} else {
					current, err = findDocumentChildByName(ctx, deps, current.ID, segment)
				}
				if err != nil {
					return err
				}
			}

			return printResult(cmd, deps, map[string]any{
				"path": currentPathString(segments),
				"id":   current.ID,
				"name": current.Name,
			})
		},
	}
	return cmd
}

func parseTreePath(raw string) []string {
	parts := strings.Split(raw, "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			segments = append(segments, trimmed)
		}
	}
	return segments
}

func currentPathString(segments []string) string {
	return strings.Join(segments, "/")
}

// The lookups walk every page rather than trusting the server's default
// page size: a single unpaginated GET silently misses nodes on parents
// with more children than one page, making "could not find" a lie.

func findDocumentRootByName(ctx context.Context, deps Dependencies, name string) (treeNodeRef, error) {
	result, err := getAllPagesWithFallback(
		ctx,
		deps.Client,
		0, 0, 0,
		getRequestCandidate{path: "/tree/document/root", opts: api.RequestOptions{}},
		getRequestCandidate{path: "/document/root", opts: api.RequestOptions{}},
	)
	if err != nil {
		return treeNodeRef{}, err
	}
	return selectTreeNodeByName(result, name, "root")
}

func findDocumentChildByName(ctx context.Context, deps Dependencies, parentID string, name string) (treeNodeRef, error) {
	result, err := getAllPagesWithFallback(
		ctx,
		deps.Client,
		0, 0, 0,
		getRequestCandidate{path: "/tree/document/children", opts: api.RequestOptions{Params: map[string]any{"parentId": parentID}}},
		getRequestCandidate{path: api.JoinPath("/document/%s/children", parentID), opts: api.RequestOptions{}},
	)
	if err != nil {
		return treeNodeRef{}, err
	}
	return selectTreeNodeByName(result, name, parentID)
}

func selectTreeNodeByName(raw any, name string, location string) (treeNodeRef, error) {
	payload, err := decodeResult[map[string]any](raw)
	if err != nil {
		return treeNodeRef{}, err
	}

	rawItems, ok := payload["items"].([]any)
	if !ok {
		return treeNodeRef{}, fmt.Errorf("tree response at %s did not include items", location)
	}

	matches := make([]treeNodeRef, 0)
	for _, item := range rawItems {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemID, _ := itemMap["id"].(string)
		if itemID == "" {
			continue
		}
		for _, itemName := range treeItemNames(itemMap) {
			if itemName == name {
				matches = append(matches, treeNodeRef{ID: itemID, Name: itemName})
				break
			}
		}
	}

	switch len(matches) {
	case 0:
		return treeNodeRef{}, fmt.Errorf("tree walk could not find %q under %s", name, location)
	case 1:
		return matches[0], nil
	default:
		return treeNodeRef{}, fmt.Errorf("tree walk found multiple matches for %q under %s", name, location)
	}
}

// treeItemNames returns every name a tree item is known by. Older
// Management APIs put a top-level name on tree items; modern ones carry
// per-culture names inside variants[] with no top-level field, which made
// matching on item["name"] silently find nothing.
func treeItemNames(item map[string]any) []string {
	names := make([]string, 0, 2)
	if name, ok := item["name"].(string); ok && name != "" {
		names = append(names, name)
	}
	variants, _ := item["variants"].([]any)
	for _, raw := range variants {
		variant, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := variant["name"].(string); ok && name != "" {
			names = append(names, name)
		}
	}
	return names
}
