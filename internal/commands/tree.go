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

			ctx := context.Background()
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

func findDocumentRootByName(ctx context.Context, deps Dependencies, name string) (treeNodeRef, error) {
	result, err := getWithFallback(
		ctx,
		deps.Client,
		getRequestCandidate{path: "/tree/document/root", opts: api.RequestOptions{}},
		getRequestCandidate{path: "/document/root", opts: api.RequestOptions{}},
	)
	if err != nil {
		return treeNodeRef{}, err
	}
	return selectTreeNodeByName(result, name, "root")
}

func findDocumentChildByName(ctx context.Context, deps Dependencies, parentID string, name string) (treeNodeRef, error) {
	result, err := getWithFallback(
		ctx,
		deps.Client,
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
		itemName, _ := itemMap["name"].(string)
		itemID, _ := itemMap["id"].(string)
		if itemName == name && itemID != "" {
			matches = append(matches, treeNodeRef{ID: itemID, Name: itemName})
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
