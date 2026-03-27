package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

func RegisterMedia(root *cobra.Command, deps Dependencies) {
	media := &cobra.Command{Use: "media", Short: "Media asset operations"}

	media.AddCommand(mediaGet(deps))
	media.AddCommand(mediaRoot(deps))
	media.AddCommand(mediaChildren(deps))
	media.AddCommand(mediaSearch(deps))
	media.AddCommand(mediaURLs(deps))
	media.AddCommand(mediaCreate(deps))
	media.AddCommand(mediaCreateFolder(deps))
	media.AddCommand(mediaUpdate(deps))
	media.AddCommand(mediaMove(deps))
	media.AddCommand(mediaDelete(deps))
	media.AddCommand(mediaTrash(deps))

	root.AddCommand(media)
}

func mediaGet(deps Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{Use: "get <id>", Short: "Get media by ID", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(context.Background(), fmt.Sprintf("/media/%s", args[0]), api.RequestOptions{Fields: fields})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	return cmd
}

func mediaRoot(deps Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{Use: "root", Short: "Get root media items", RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(context.Background(), "/media/root", api.RequestOptions{Fields: fields})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	return cmd
}

func mediaChildren(deps Dependencies) *cobra.Command {
	var fields string
	cmd := &cobra.Command{Use: "children <id>", Short: "Get child media items", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(context.Background(), fmt.Sprintf("/media/%s/children", args[0]), api.RequestOptions{Fields: fields})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&fields, "fields", "", "Limit response fields")
	return cmd
}

func mediaSearch(deps Dependencies) *cobra.Command {
	var paramsRaw string
	var query string
	var skip int
	var take int

	cmd := &cobra.Command{Use: "search", Short: "Search media items", RunE: func(cmd *cobra.Command, args []string) error {
		params, err := parseParams(paramsRaw)
		if err != nil {
			return err
		}
		if params == nil {
			if query == "" {
				return fmt.Errorf("media search requires either --params or --query")
			}
			params = map[string]any{"query": query}
			if skip >= 0 {
				params["skip"] = skip
			}
			if take >= 0 {
				params["take"] = take
			}
		}

		result, err := getWithFallback(
			context.Background(),
			deps.Client,
			getRequestCandidate{path: "/item/media/search", opts: api.RequestOptions{Params: params}},
			getRequestCandidate{path: "/media/search", opts: api.RequestOptions{Params: params}},
		)
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}

	cmd.Flags().StringVar(&paramsRaw, "params", "", "Search parameters as JSON")
	cmd.Flags().StringVar(&query, "query", "", "Search query")
	cmd.Flags().IntVar(&skip, "skip", -1, "Skip count")
	cmd.Flags().IntVar(&take, "take", -1, "Take count")
	return cmd
}

func mediaURLs(deps Dependencies) *cobra.Command {
	return &cobra.Command{Use: "urls <id>", Short: "Get media URLs", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Get(context.Background(), fmt.Sprintf("/media/%s/urls", args[0]), api.RequestOptions{})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
}

func mediaCreate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	cmd := &cobra.Command{Use: "create", Short: "Create media from JSON payload", RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireValue("--json", jsonPayload); err != nil {
			return err
		}
		body, err := parsePayload(jsonPayload)
		if err != nil {
			return err
		}
		result, err := deps.Client.Post(context.Background(), "/media", body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create payload as JSON")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func mediaCreateFolder(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var parent string
	var dryRun bool
	cmd := &cobra.Command{Use: "create-folder [name]", Short: "Create media folder", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var body map[string]any
		var err error
		if jsonPayload != "" {
			body, err = parsePayload(jsonPayload)
		} else {
			if len(args) == 0 || args[0] == "" {
				return fmt.Errorf("create-folder requires [name] when --json is not provided")
			}
			if err := requireValue("--parent", parent); err != nil {
				return err
			}
			body = map[string]any{"name": args[0], "parent": map[string]any{"id": parent}}
		}
		if err != nil {
			return err
		}
		result, err := deps.Client.Post(context.Background(), "/media/folder", body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create-folder payload as JSON")
	cmd.Flags().StringVar(&parent, "parent", "", "Target parent ID")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func mediaUpdate(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var dryRun bool
	cmd := &cobra.Command{Use: "update <id>", Short: "Update media item", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireValue("--json", jsonPayload); err != nil {
			return err
		}
		body, err := parsePayload(jsonPayload)
		if err != nil {
			return err
		}
		result, err := deps.Client.Put(context.Background(), fmt.Sprintf("/media/%s", args[0]), body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Update payload as JSON")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func mediaMove(deps Dependencies) *cobra.Command {
	var jsonPayload string
	var to string
	var dryRun bool
	cmd := &cobra.Command{Use: "move <id>", Short: "Move media item", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var body map[string]any
		var err error
		if jsonPayload != "" {
			body, err = parsePayload(jsonPayload)
		} else {
			if err := requireValue("--to", to); err != nil {
				return err
			}
			body = map[string]any{"target": map[string]any{"id": to}}
		}
		if err != nil {
			return err
		}
		result, err := deps.Client.Post(context.Background(), fmt.Sprintf("/media/%s/move", args[0]), body, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Move payload as JSON")
	cmd.Flags().StringVar(&to, "to", "", "Target parent ID")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func mediaDelete(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{Use: "delete <id>", Short: "Delete media item", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Delete(context.Background(), fmt.Sprintf("/media/%s", args[0]), api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}

func mediaTrash(deps Dependencies) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{Use: "trash <id>", Short: "Move media item to recycle bin", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		result, err := deps.Client.Post(context.Background(), fmt.Sprintf("/media/%s/move-to-recycle-bin", args[0]), map[string]any{}, api.RequestOptions{DryRun: dryRun})
		if err != nil {
			return err
		}
		return printResult(cmd, deps, result)
	}}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate request without executing")
	return cmd
}
