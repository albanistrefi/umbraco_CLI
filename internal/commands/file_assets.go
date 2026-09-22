package commands

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

// fileAssetSpec parameterizes the command surface of the file-based assets
// (partial views, scripts, stylesheets, static files). These resources are
// keyed by their virtual file-system path, not by a GUID, so none of the
// ID-based archetypes fit: the path is both the identity and the tree
// coordinate. Everything else — pagination, --dry-run, force gating,
// --fields, the output contract — is the house contract.
type fileAssetSpec struct {
	Use      string // command group name, e.g. "partial-view"
	Resource string // API resource segment, e.g. "partial-view"
	Display  string // human name used in help text, e.g. "partial view"
	// Extension is the conventional file extension, used in help text only.
	Extension string
	// ReadOnly drops every mutation; static files are served from disk and
	// the Management API exposes no write side for them.
	ReadOnly bool
	// Snippets adds the partial-view-only snippet catalogue commands.
	Snippets bool
	// ItemEndpoint, when non-empty, is the /item/<resource> lookup used by
	// read-only groups whose resource has no GET /<resource>/{path} route.
	ItemEndpoint string
	// Long is the group's "Task → command" help map.
	Long string
}

func RegisterPartialView(root *cobra.Command, deps Dependencies) {
	registerFileAssetGroup(root, deps, fileAssetSpec{
		Use: "partial-view", Resource: "partial-view", Display: "partial view",
		Extension: ".cshtml", Snippets: true,
		Long: `Partial view operations. Partial views are keyed by their path under /Views/Partials, not by a GUID.

Task → command:
  See the folders and files at the root            partial-view list
  See what is inside a folder                      partial-view children /Blog
  Read a file's content                            partial-view get /Blog/Header.cshtml
  Save a file to disk verbatim                     partial-view get /Blog/Header.cshtml --out ./Header.cshtml
  Create a folder                                  partial-view create-folder --path / --name Blog
  Create a file                                    partial-view create --path /Blog --name Header.cshtml --content-file ./Header.cshtml
  Overwrite a file's content                       partial-view update /Blog/Header.cshtml --content-file ./Header.cshtml
  Rename a file (keeping its folder)               partial-view rename /Blog/Header.cshtml --name Top.cshtml
  Delete a file or an empty folder                 partial-view delete /Blog/Header.cshtml --force; partial-view delete-folder /Blog --force
  Start from a built-in snippet                    partial-view snippets; partial-view snippet Breadcrumb`,
	})
}

func RegisterScript(root *cobra.Command, deps Dependencies) {
	registerFileAssetGroup(root, deps, fileAssetSpec{
		Use: "script", Resource: "script", Display: "script",
		Extension: ".js",
		Long: `Script operations. Scripts are keyed by their path under /wwwroot/scripts, not by a GUID.

Task → command:
  See the folders and files at the root            script list
  See what is inside a folder                      script children /vendor
  Read a file's content                            script get /vendor/app.js
  Save a file to disk verbatim                     script get /vendor/app.js --out ./app.js
  Create a folder                                  script create-folder --path / --name vendor
  Create a file                                    script create --path /vendor --name app.js --content-file ./app.js
  Overwrite a file's content                       script update /vendor/app.js --content-file ./app.js
  Rename a file (keeping its folder)               script rename /vendor/app.js --name main.js
  Delete a file or an empty folder                 script delete /vendor/app.js --force; script delete-folder /vendor --force`,
	})
}

func RegisterStylesheet(root *cobra.Command, deps Dependencies) {
	registerFileAssetGroup(root, deps, fileAssetSpec{
		Use: "stylesheet", Resource: "stylesheet", Display: "stylesheet",
		Extension: ".css",
		Long: `Stylesheet operations. Stylesheets are keyed by their path under /wwwroot/css, not by a GUID.

Task → command:
  See the folders and files at the root            stylesheet list
  See what is inside a folder                      stylesheet children /theme
  Read a file's content                            stylesheet get /theme/site.css
  Save a file to disk verbatim                     stylesheet get /theme/site.css --out ./site.css
  Create a folder                                  stylesheet create-folder --path / --name theme
  Create a file                                    stylesheet create --path /theme --name site.css --content-file ./site.css
  Overwrite a file's content                       stylesheet update /theme/site.css --content-file ./site.css
  Rename a file (keeping its folder)               stylesheet rename /theme/site.css --name main.css
  Delete a file or an empty folder                 stylesheet delete /theme/site.css --force; stylesheet delete-folder /theme --force`,
	})
}

func RegisterStaticFile(root *cobra.Command, deps Dependencies) {
	registerFileAssetGroup(root, deps, fileAssetSpec{
		Use: "static-file", Resource: "static-file", Display: "static file",
		ReadOnly: true, ItemEndpoint: "/item/static-file",
		Long: `Static file operations (read only). Static files are served from disk; the Management API exposes no write side for them.

Task → command:
  See the folders and files at the root            static-file list
  See what is inside a folder                      static-file children /wwwroot
  Look one path up (name, folder, isFolder)        static-file get /wwwroot/favicon.ico`,
	})
}

func registerFileAssetGroup(root *cobra.Command, deps Dependencies, spec fileAssetSpec) {
	group := &cobra.Command{
		Use:   spec.Use,
		Short: fmt.Sprintf("%s operations", strings.ToUpper(spec.Display[:1])+spec.Display[1:]),
		Long:  spec.Long,
	}
	group.AddCommand(fileAssetList(deps, spec))
	group.AddCommand(fileAssetChildren(deps, spec))
	group.AddCommand(fileAssetGet(deps, spec))
	if !spec.ReadOnly {
		group.AddCommand(fileAssetCreate(deps, spec))
		group.AddCommand(fileAssetUpdate(deps, spec))
		group.AddCommand(fileAssetRename(deps, spec))
		group.AddCommand(fileAssetDelete(deps, spec))
		group.AddCommand(fileAssetCreateFolder(deps, spec))
		group.AddCommand(fileAssetDeleteFolder(deps, spec))
	}
	if spec.Snippets {
		group.AddCommand(fileAssetSnippets(deps, spec))
		group.AddCommand(fileAssetSnippet(deps, spec))
	}
	root.AddCommand(group)
}

// --- path encoding -------------------------------------------------------
//
// Two encodings are needed, and they are not interchangeable. Verified
// against Umbraco 18.1:
//
//   - GET/PUT/DELETE /<resource>/{path} and /<resource>/folder/{path} are
//     catch-all routes: the path is sent with its separators intact and only
//     the individual segments percent-escaped.
//   - PUT /<resource>/{path}/rename is NOT a catch-all — {path} must arrive
//     as a single route segment. Kestrel decodes percent-escapes once before
//     routing, so the separators have to be encoded twice (%252F) to survive
//     as one segment; the model binder decodes the remaining %2F.
//
// Both reject "." and ".." segments outright rather than letting a caller
// rewrite the route through path normalization.

func normalizeFilePath(path string) (string, error) {
	trimmed := strings.Trim(strings.TrimSpace(path), "/")
	if trimmed == "" {
		return "", fmt.Errorf("path must not be empty; pass a path like /Blog/Header.cshtml")
	}
	// Umbraco's virtual paths always use forward slashes, but a
	// Windows-hosted instance treats a backslash as a separator too — which
	// would let "vendor\..\secret.js" slip past the segment checks below,
	// since they only split on "/". Backslashes are rejected outright.
	if strings.ContainsRune(trimmed, '\\') {
		return "", fmt.Errorf(`invalid path %q: backslashes are not allowed, use / as the separator`, path)
	}
	for _, segment := range strings.Split(trimmed, "/") {
		if segment == "" {
			return "", fmt.Errorf("invalid path %q: it contains an empty segment", path)
		}
		if strings.Trim(segment, ".") == "" {
			return "", fmt.Errorf("invalid path %q: relative segments (. and ..) are not allowed", path)
		}
	}
	return trimmed, nil
}

// escapeFilePath escapes each segment but keeps the separators, for the
// catch-all {path} routes.
func escapeFilePath(path string) (string, error) {
	normalized, err := normalizeFilePath(path)
	if err != nil {
		return "", err
	}
	segments := strings.Split(normalized, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/"), nil
}

// escapeFilePathSegment collapses the path into one doubly-escaped route
// segment, for the rename route.
func escapeFilePathSegment(path string) (string, error) {
	escaped, err := escapeFilePath(path)
	if err != nil {
		return "", err
	}
	return url.PathEscape(strings.ReplaceAll(escaped, "/", "%2F")), nil
}

// apiFilePath is the canonical server-side form of a path: leading slash,
// no trailing slash. It is what create/create-folder put in parent.path and
// what the tree endpoints echo back.
func apiFilePath(path string) (string, error) {
	normalized, err := normalizeFilePath(path)
	if err != nil {
		return "", err
	}
	return "/" + normalized, nil
}

// isRootFilePath reports whether a --path or children argument names the
// root of the tree rather than a folder inside it.
func isRootFilePath(path string) bool {
	return strings.Trim(strings.TrimSpace(path), "/") == ""
}

// filePathParent resolves a parent folder argument into the request body's
// parent envelope. The root is expressed by omitting parent entirely.
func filePathParent(path string) (map[string]any, error) {
	if isRootFilePath(path) {
		return nil, nil
	}
	parent, err := apiFilePath(path)
	if err != nil {
		return nil, err
	}
	return map[string]any{"path": parent}, nil
}

// createdFilePath is the full path of a file or folder created under parent.
func createdFilePath(parentPath string, name string) string {
	parent, err := apiFilePath(parentPath)
	if err != nil {
		return "/" + name
	}
	return parent + "/" + name
}

// --- reads ---------------------------------------------------------------

func fileAssetList(deps Dependencies, spec fileAssetSpec) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "list",
		Short: fmt.Sprintf("List %ss and folders at the root (paginated; --skip/--take/--all)", spec.Display),
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/tree/" + spec.Resource + "/root", opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func fileAssetChildren(deps Dependencies, spec fileAssetSpec) *cobra.Command {
	cmd := collectionCommand(deps, collectionSpec{
		Use:   "children <path>",
		Short: fmt.Sprintf("List %ss and folders inside a folder (paginated; --skip/--take/--all)", spec.Display),
		NArgs: 1,
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(cmd, args); err != nil {
				return err
			}
			if isRootFilePath(args[0]) {
				return nil
			}
			_, err := normalizeFilePath(args[0])
			return err
		},
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			// "/" means the root, which the tree endpoint takes as
			// parentPath=/ rather than as a folder path.
			parent := "/"
			if !isRootFilePath(args[0]) {
				normalized, err := apiFilePath(args[0])
				if err == nil {
					parent = normalized
				}
			}
			return []getRequestCandidate{
				{path: "/tree/" + spec.Resource + "/children", opts: api.RequestOptions{Params: withParam(params, "parentPath", parent)}},
			}
		},
	})
	return cmd
}

func fileAssetGet(deps Dependencies, spec fileAssetSpec) *cobra.Command {
	var fields string
	var outFile string
	short := fmt.Sprintf("Get a %s by path, including its content", spec.Display)
	long := fmt.Sprintf("Fetches a %s by its path (for example /Blog/Header%s). --out writes the content to disk verbatim, so a file can be round-tripped without shell quoting.", spec.Display, spec.Extension)
	if spec.ReadOnly {
		short = fmt.Sprintf("Get a %s entry by path", spec.Display)
		long = fmt.Sprintf("Looks up one %s by its path. The Management API returns the entry (name, path, parent, isFolder) — static file content is not served through it.", spec.Display)
	}
	cmd := &cobra.Command{
		Use:   "get <path>",
		Short: short,
		Long:  long,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := fileAssetFetch(cmd, deps, spec, args[0], fields)
			if err != nil {
				return err
			}
			if outFile != "" {
				written, err := writeFileAssetContent(outFile, result)
				if err != nil {
					return err
				}
				return printResult(cmd, deps, map[string]any{"out": outFile, "bytes": written})
			}
			return printResult(cmd, deps, applyFieldsProjection(result, fields))
		},
	}
	addFieldsFlag(cmd, &fields)
	if !spec.ReadOnly {
		cmd.Flags().StringVar(&outFile, "out", "", "Write the content to this file verbatim and print a summary instead of the body")
	}
	return cmd
}

func fileAssetFetch(cmd *cobra.Command, deps Dependencies, spec fileAssetSpec, path string, fields string) (any, error) {
	if spec.ItemEndpoint != "" {
		normalized, err := apiFilePath(path)
		if err != nil {
			return nil, err
		}
		result, err := deps.Client.Get(cmd.Context(), spec.ItemEndpoint, api.RequestOptions{
			Fields: fields,
			Params: map[string]any{"path": normalized},
		})
		if err != nil {
			return nil, err
		}
		return unwrapFileAssetItem(spec, normalized, result)
	}
	escaped, err := escapeFilePath(path)
	if err != nil {
		return nil, err
	}
	return deps.Client.Get(cmd.Context(), "/"+spec.Resource+"/"+escaped, api.RequestOptions{Fields: fields})
}

// unwrapFileAssetItem turns the /item/<resource> array into the single
// entry that was asked for. The endpoint takes a repeatable path parameter
// and answers with an array, so a get of one path would otherwise print a
// one-element array — and, worse, report success with [] for a path that
// does not exist.
func unwrapFileAssetItem(spec fileAssetSpec, path string, result any) (any, error) {
	items, ok := result.([]any)
	if !ok {
		// A single object means the endpoint answered in its non-array
		// shape; nothing to unwrap.
		return result, nil
	}
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if itemPath, ok := object["path"].(string); ok && itemPath == path {
			return object, nil
		}
	}
	return nil, fmt.Errorf("%s %s not found", spec.Display, path)
}

// writeFileAssetContent writes the content field of a fetched file asset to
// disk verbatim, so `get --out` round-trips into `create/update
// --content-file` byte for byte.
func writeFileAssetContent(outFile string, result any) (int, error) {
	object, ok := result.(map[string]any)
	if !ok {
		return 0, fmt.Errorf("--out needs a response with a content field, got %s", jsonShapeName(result))
	}
	content, ok := object["content"].(string)
	if !ok {
		return 0, fmt.Errorf("--out needs a response with a content field; this response has none")
	}
	if dir := filepath.Dir(outFile); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return 0, fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(outFile, []byte(content), 0o644); err != nil {
		return 0, fmt.Errorf("failed to write %s: %w", outFile, err)
	}
	return len(content), nil
}

func fileAssetSnippets(deps Dependencies, spec fileAssetSpec) *cobra.Command {
	return collectionCommand(deps, collectionSpec{
		Use:   "snippets",
		Short: "List the built-in partial view snippets (paginated; --skip/--take/--all)",
		Endpoints: func(args []string, params map[string]any) []getRequestCandidate {
			return []getRequestCandidate{
				{path: "/" + spec.Resource + "/snippet", opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func fileAssetSnippet(deps Dependencies, spec fileAssetSpec) *cobra.Command {
	return getCommand(deps, getSpec{
		Use:   "snippet <id>",
		Short: "Get a built-in partial view snippet, including its content",
		Long:  "Fetches one snippet by its id (as listed by `partial-view snippets`). Its content is the starting point Umbraco offers in the backoffice when creating a partial view.",
		Path: func(args []string) string {
			return api.JoinPath("/"+spec.Resource+"/snippet/%s", args[0])
		},
	})
}

// --- content resolution --------------------------------------------------

// resolveFileAssetContent enforces the --content / --content-file pair:
// exactly one, with --content-file read verbatim so agents never have to
// quote a file through the shell.
func resolveFileAssetContent(cmd *cobra.Command, content string, contentFile string, required bool) (string, error) {
	hasContent := cmd.Flags().Changed("content")
	hasFile := strings.TrimSpace(contentFile) != ""
	switch {
	case hasContent && hasFile:
		return "", fmt.Errorf("%s takes either --content or --content-file, not both", cmd.CommandPath())
	case hasFile:
		raw, err := os.ReadFile(contentFile)
		if err != nil {
			return "", fmt.Errorf("failed to read --content-file %s: %w", contentFile, err)
		}
		return string(raw), nil
	case hasContent:
		return content, nil
	case required:
		return "", fmt.Errorf("%s requires --content or --content-file", cmd.CommandPath())
	default:
		return "", nil
	}
}

// fileAssetCreateResult echoes the identity of what was created. The
// Management API answers a create with the bare file name, which does not
// say where the file landed — the full path is what every follow-up command
// (get/update/rename/delete) takes, so it is always reported.
func fileAssetCreateResult(result any, name string, path string, dryRun bool) any {
	if dryRun {
		return result
	}
	payload := map[string]any{"name": name, "path": path}
	if object, ok := result.(map[string]any); ok {
		for key, value := range object {
			if _, exists := payload[key]; !exists {
				payload[key] = value
			}
		}
	}
	return payload
}

func addContentFlags(cmd *cobra.Command, content *string, contentFile *string) {
	cmd.Flags().StringVar(content, "content", "", "File content as text")
	cmd.Flags().StringVar(contentFile, "content-file", "", "Read the file content verbatim from this local file")
}

// --- mutations -----------------------------------------------------------

func fileAssetCreate(deps Dependencies, spec fileAssetSpec) *cobra.Command {
	var parentPath string
	var name string
	var content string
	var contentFile string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "create --path <folder> --name <file>",
		Short: fmt.Sprintf("Create a %s", spec.Display),
		Long:  fmt.Sprintf("Creates a %s named --name inside the folder --path (use / for the root). The content comes from --content or, for anything with quotes or newlines, --content-file.", spec.Display),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireValue("--name", name); err != nil {
				return err
			}
			if err := requireValue("--path", parentPath); err != nil {
				return err
			}
			resolved, err := resolveFileAssetContent(cmd, content, contentFile, true)
			if err != nil {
				return err
			}
			parent, err := filePathParent(parentPath)
			if err != nil {
				return err
			}
			body := map[string]any{"name": name, "content": resolved}
			if parent != nil {
				body["parent"] = parent
			}
			result, err := deps.Client.Post(cmd.Context(), "/"+spec.Resource, body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, fileAssetCreateResult(result, name, createdFilePath(parentPath, name), dryRun))
		},
	}
	cmd.Flags().StringVar(&parentPath, "path", "/", "Folder to create the file in (/ for the root)")
	cmd.Flags().StringVar(&name, "name", "", fmt.Sprintf("File name, including the %s extension", spec.Extension))
	addContentFlags(cmd, &content, &contentFile)
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func fileAssetUpdate(deps Dependencies, spec fileAssetSpec) *cobra.Command {
	var content string
	var contentFile string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "update <path>",
		Short: fmt.Sprintf("Replace a %s's content", spec.Display),
		Long:  fmt.Sprintf("Replaces the whole content of a %s. The Management API update model carries content only — there is no partial update, so pass the full file (--content-file round-trips `get --out`).", spec.Display),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			escaped, err := escapeFilePath(args[0])
			if err != nil {
				return err
			}
			resolved, err := resolveFileAssetContent(cmd, content, contentFile, true)
			if err != nil {
				return err
			}
			result, err := deps.Client.Put(cmd.Context(), "/"+spec.Resource+"/"+escaped, map[string]any{"content": resolved}, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "updated", result, dryRun)
		},
	}
	addContentFlags(cmd, &content, &contentFile)
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func fileAssetRename(deps Dependencies, spec fileAssetSpec) *cobra.Command {
	var name string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "rename <path> --name <new-name>",
		Short: fmt.Sprintf("Rename a %s, keeping it in its folder", spec.Display),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireValue("--name", name); err != nil {
				return err
			}
			segment, err := escapeFilePathSegment(args[0])
			if err != nil {
				return err
			}
			result, err := deps.Client.Put(cmd.Context(), "/"+spec.Resource+"/"+segment+"/rename", map[string]any{"name": name}, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "renamed", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "New file name, including the extension")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func fileAssetDelete(deps Dependencies, spec fileAssetSpec) *cobra.Command {
	var force bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "delete <path>",
		Short: fmt.Sprintf("Permanently delete a %s", spec.Display),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireForceOrDryRun(cmd, "permanently deletes", force, dryRun); err != nil {
				return err
			}
			escaped, err := escapeFilePath(args[0])
			if err != nil {
				return err
			}
			result, err := deps.Client.Delete(cmd.Context(), "/"+spec.Resource+"/"+escaped, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "deleted", result, dryRun)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Confirm permanent deletion")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func fileAssetCreateFolder(deps Dependencies, spec fileAssetSpec) *cobra.Command {
	var parentPath string
	var name string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "create-folder --path <parent> --name <name>",
		Short: fmt.Sprintf("Create a %s folder", spec.Display),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireValue("--name", name); err != nil {
				return err
			}
			if err := requireValue("--path", parentPath); err != nil {
				return err
			}
			parent, err := filePathParent(parentPath)
			if err != nil {
				return err
			}
			body := map[string]any{"name": name}
			if parent != nil {
				body["parent"] = parent
			}
			result, err := deps.Client.Post(cmd.Context(), "/"+spec.Resource+"/folder", body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printResult(cmd, deps, fileAssetCreateResult(result, name, createdFilePath(parentPath, name), dryRun))
		},
	}
	cmd.Flags().StringVar(&parentPath, "path", "/", "Parent folder (/ for the root)")
	cmd.Flags().StringVar(&name, "name", "", "Folder name")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}

func fileAssetDeleteFolder(deps Dependencies, spec fileAssetSpec) *cobra.Command {
	var force bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "delete-folder <path>",
		Short: fmt.Sprintf("Permanently delete an empty %s folder", spec.Display),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireForceOrDryRun(cmd, "permanently deletes", force, dryRun); err != nil {
				return err
			}
			escaped, err := escapeFilePath(args[0])
			if err != nil {
				return err
			}
			result, err := deps.Client.Delete(cmd.Context(), "/"+spec.Resource+"/folder/"+escaped, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return printMutationResult(cmd, deps, "deleted", result, dryRun)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Confirm permanent deletion")
	addDryRunFlag(cmd, &dryRun)
	return cmd
}
