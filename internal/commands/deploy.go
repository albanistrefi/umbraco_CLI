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
		Short: "Deployment observation and schema application (watch, status, apply)",
	}
	deploy.AddCommand(deployWatch(deps))
	deploy.AddCommand(deployStatus(deps))
	deploy.AddCommand(deployApply(deps))
	root.AddCommand(deploy)
}
