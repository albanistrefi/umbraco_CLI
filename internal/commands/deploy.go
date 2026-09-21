package commands

import (
	"github.com/spf13/cobra"
)

// RegisterDeploy wires the effect-based deployment observation commands.
// There is deliberately no deployment-status API underneath these — a
// status API reports what a pipeline believes happened, while these
// commands observe what the target environment is actually doing (app
// recycles, endpoint availability, index rebuilds). That also makes them
// host-agnostic: they behave identically on Umbraco Cloud and on-prem, and
// they are strictly read-only against the environment.
func RegisterDeploy(root *cobra.Command, deps Dependencies) {
	deploy := &cobra.Command{
		Use:   "deploy",
		Short: "Deployment observation, schema application, and content transfer (watch, status, apply, transfer, queue)",
		Long: `Deployment commands.

Task → command:
  Watch an environment while a deploy lands              deploy watch
  Compare .uda schema artifacts with an environment      deploy status --uda-dir <dir>
  Apply .uda schema ("Update schema")                    deploy apply --uda-dir <dir> --dry-run
  Move content to the next environment, GUIDs intact     deploy transfer --node <id> [--descendants] --dry-run   (Umbraco Deploy required)
  Queue content and transfer it together                 deploy queue add <id>; deploy queue list; deploy transfer --queue --force`,
	}
	deploy.AddCommand(deployWatch(deps))
	deploy.AddCommand(deployStatus(deps))
	deploy.AddCommand(deployApply(deps))
	deploy.AddCommand(deployTransfer(deps))
	deploy.AddCommand(deployQueue(deps))
	root.AddCommand(deploy)
}
