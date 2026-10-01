package commands

import (
	"strings"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/commands/cmdkit"
)

func RegisterLanguage(root *cobra.Command, deps cmdkit.Dependencies) {
	language := &cobra.Command{
		Use:   "language",
		Short: "Language and culture management for variant content",
		Long:  "Manage the languages content can vary by. 'language cultures' lists every ISO culture the server knows, for picking valid isoCode values. Languages are addressed by isoCode (e.g. en-US), not GUID.",
	}
	language.AddCommand(languageList(deps))
	language.AddCommand(languageGet(deps))
	language.AddCommand(languageCreate(deps))
	language.AddCommand(languageUpdate(deps))
	language.AddCommand(languageDelete(deps))
	language.AddCommand(languageDefault(deps))
	language.AddCommand(languageCultures(deps))
	root.AddCommand(language)
}

func languageList(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.CollectionCommand(deps, cmdkit.CollectionSpec{
		Use:   "list",
		Short: "List configured languages (paginated; --skip/--take/--all)",
		Endpoints: func(args []string, params map[string]any) []cmdkit.GetRequestCandidate {
			return []cmdkit.GetRequestCandidate{
				{Path: "/language", Opts: api.RequestOptions{Params: params}},
			}
		},
	})
}

func languageGet(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.GetCommand(deps, cmdkit.GetSpec{
		Use:   "get <iso-code>",
		Short: "Get a language by ISO code (e.g. en-US)",
		Path:  func(args []string) string { return api.JoinPath("/language/%s", args[0]) },
	})
}

func languageCreate(deps cmdkit.Dependencies) *cobra.Command {
	var jsonPayload string
	var isoCode string
	var name string
	var isDefault bool
	var isMandatory bool
	var fallback string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a language",
		Long:  "POST /language. Either pass the full payload via --json, or use the convenience flags (--iso-code and --name required). Valid ISO codes come from 'language cultures'.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var body map[string]any
			if strings.TrimSpace(jsonPayload) != "" {
				parsed, err := cmdkit.ParsePayload(jsonPayload)
				if err != nil {
					return err
				}
				body = parsed
			} else {
				if err := cmdkit.RequireValue("--iso-code", isoCode); err != nil {
					return err
				}
				if err := cmdkit.RequireValue("--name", name); err != nil {
					return err
				}
				body = map[string]any{
					"isoCode":     isoCode,
					"name":        name,
					"isDefault":   isDefault,
					"isMandatory": isMandatory,
				}
				if strings.TrimSpace(fallback) != "" {
					body["fallbackIsoCode"] = fallback
				}
			}
			result, err := deps.Client.Post(cmd.Context(), "/language", body, api.RequestOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			return cmdkit.PrintMutationResult(cmd, deps, "created", result, dryRun)
		},
	}
	cmd.Flags().StringVar(&jsonPayload, "json", "", "Create payload as JSON")
	cmd.Flags().StringVar(&isoCode, "iso-code", "", "Language ISO code (e.g. da-DK)")
	cmd.Flags().StringVar(&name, "name", "", "Display name (e.g. Danish)")
	cmd.Flags().BoolVar(&isDefault, "default", false, "Make this the default language")
	cmd.Flags().BoolVar(&isMandatory, "mandatory", false, "Require this language before content can publish")
	cmd.Flags().StringVar(&fallback, "fallback", "", "Fallback language ISO code")
	cmdkit.AddDryRunFlag(cmd, &dryRun)
	return cmd
}

func languageUpdate(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.UpdateCommand(deps, cmdkit.UpdateSpec{
		Use:   "update <iso-code>",
		Short: "Update a language",
		Path:  func(args []string) string { return api.JoinPath("/language/%s", args[0]) },
		// The update model has no isoCode field (it lives in the path);
		// a merge against the GET response would otherwise echo it back.
		NormalizeMerged: cmdkit.StripFields("isoCode"),
	})
}

func languageDelete(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.DeleteCommand(deps, cmdkit.DeleteSpec{
		Use:   "delete <iso-code>",
		Short: "Permanently delete a language (content variants for it become unreachable)",
		Path: func(args []string) string {
			return api.JoinPath("/language/%s", args[0])
		},
	})
}

func languageDefault(deps cmdkit.Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "default",
		Short: "Get the default language",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := deps.Client.Get(cmd.Context(), "/item/language/default", api.RequestOptions{})
			if err != nil {
				return err
			}
			return cmdkit.PrintResult(cmd, deps, result)
		},
	}
}

func languageCultures(deps cmdkit.Dependencies) *cobra.Command {
	return cmdkit.CollectionCommand(deps, cmdkit.CollectionSpec{
		Use:   "cultures",
		Short: "List the ISO cultures available for new languages (paginated; --skip/--take/--all)",
		Endpoints: func(args []string, params map[string]any) []cmdkit.GetRequestCandidate {
			return []cmdkit.GetRequestCandidate{
				{Path: "/culture", Opts: api.RequestOptions{Params: params}},
			}
		},
	})
}
