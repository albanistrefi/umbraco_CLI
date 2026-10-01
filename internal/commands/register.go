package commands

import (
	"github.com/spf13/cobra"

	"umbraco-cli/internal/commands/automate"
	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/commands/deploy"
	"umbraco-cli/internal/commands/engage"
	"umbraco-cli/internal/commands/forms"
)

// RegisterAll attaches every command group to root, core and add-on alike. It
// is the single registration list: the production root in internal/cli and the
// full-tree roots built by the tests both call it, so a group added here cannot
// be missing from either. Add-ons live in their own packages and are wired in
// through their Register functions.
func RegisterAll(root *cobra.Command, deps cmdkit.Dependencies) {
	RegisterDocument(root, deps)
	RegisterElement(root, deps)
	RegisterBlueprint(root, deps)
	RegisterDictionary(root, deps)
	RegisterMedia(root, deps)
	RegisterDoctype(root, deps)
	RegisterDatatype(root, deps)
	RegisterTemplate(root, deps)
	RegisterPartialView(root, deps)
	RegisterScript(root, deps)
	RegisterStylesheet(root, deps)
	RegisterStaticFile(root, deps)
	forms.Register(root, deps)
	RegisterModelsBuilder(root, deps)
	RegisterMember(root, deps)
	RegisterMemberGroup(root, deps)
	RegisterWebhook(root, deps)
	RegisterLanguage(root, deps)
	RegisterTag(root, deps)
	RegisterUser(root, deps)
	RegisterUserGroup(root, deps)
	RegisterUserData(root, deps)
	RegisterLogs(root, deps)
	RegisterServer(root, deps)
	RegisterHealth(root, deps)
	deploy.Register(root, deps)
	RegisterPublishedCache(root, deps)
	RegisterRedirect(root, deps)
	RegisterIndexer(root, deps)
	RegisterSearcher(root, deps)
	RegisterRelation(root, deps)
	RegisterMediaType(root, deps)
	RegisterMemberType(root, deps)
	RegisterTree(root, deps)
	RegisterAPI(root, deps)
	RegisterAuth(root, deps)
	automate.Register(root, deps)
	engage.Register(root, deps)
	RegisterSchema(root, deps)
	RegisterGenerateSkills(root, deps)
}
