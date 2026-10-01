// Code in this package predates the cmdkit split and still calls the shared
// toolkit by its original unexported names. These aliases keep those call
// sites unchanged; add-on packages import cmdkit directly and never this
// package.

package commands

import (
	"umbraco-cli/internal/commands/cmdkit"
)

type (
	backupEnvelope = cmdkit.BackupEnvelope
)
