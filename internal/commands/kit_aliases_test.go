// Code in this package predates the cmdkit/cmdtest split and still uses the
// shared toolkit and test harness by their original unexported names. These
// aliases keep those call sites unchanged; add-on packages import cmdkit and
// cmdtest directly and never this package.

package commands

import (
	"umbraco-cli/internal/commands/cmdkit"
	"umbraco-cli/internal/commands/cmdtest"
)

type (
	backupEnvelope       = cmdkit.BackupEnvelope
	endpointRoundTripper = cmdtest.RoundTripper
)

var (
	assertQueryValue     = cmdtest.AssertQueryValue
	batchExitCode        = cmdtest.ExitCode
	datatypeDeps         = cmdtest.ClientDeps
	datatypeJSONResponse = cmdtest.JSONResponse
	endpointDeps         = cmdtest.Deps
	endpointJSONResponse = cmdtest.JSONResponse
	endpointNoContent    = cmdtest.NoContent
	execute              = cmdtest.Execute
	executeWithErr       = cmdtest.ExecuteWithErr
	findChildCommand     = cmdtest.FindChildCommand
	makeDeps             = cmdtest.MakeDeps
	tokenOr404           = cmdtest.TokenOr404
)
