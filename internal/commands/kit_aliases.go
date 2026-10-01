// Code in this package predates the cmdkit/cmdtest split and still uses the
// shared toolkit and test harness by their original unexported names. These
// aliases keep those call sites unchanged; add-on packages import cmdkit and
// cmdtest directly and never this package.

package commands

import (
	"umbraco-cli/internal/commands/cmdkit"
)

const (
	logViewerLogPath = cmdkit.LogViewerLogPath
)

type (
	Dependencies        = cmdkit.Dependencies
	collectionSpec      = cmdkit.CollectionSpec
	createSpec          = cmdkit.CreateSpec
	deleteSpec          = cmdkit.DeleteSpec
	getRequestCandidate = cmdkit.GetRequestCandidate
	getSpec             = cmdkit.GetSpec
	readTriageOptions   = cmdkit.ReadTriageOptions
	updateSpec          = cmdkit.UpdateSpec
)

var (
	addDryRunFlag           = cmdkit.AddDryRunFlag
	addFieldsFlag           = cmdkit.AddFieldsFlag
	addPaginationFlags      = cmdkit.AddPaginationFlags
	addReadTriageFlags      = cmdkit.AddReadTriageFlags
	applyFieldsProjection   = cmdkit.ApplyFieldsProjection
	applyPaginationParams   = cmdkit.ApplyPaginationParams
	applyReadTriage         = cmdkit.ApplyReadTriage
	collectionCommand       = cmdkit.CollectionCommand
	createCommand           = cmdkit.CreateCommand
	createResult            = cmdkit.CreateResult
	deleteCommand           = cmdkit.DeleteCommand
	ensurePayloadID         = cmdkit.EnsurePayloadID
	getAllPagesWithFallback = cmdkit.GetAllPagesWithFallback
	getCommand              = cmdkit.GetCommand
	getWithFallback         = cmdkit.GetWithFallback
	indexerHealthStatus     = cmdkit.IndexerHealthStatus
	isAPIStatus             = cmdkit.IsAPIStatus
	mergeAliasPayload       = cmdkit.MergeAliasPayload
	mergeParams             = cmdkit.MergeParams
	optionalBody            = cmdkit.OptionalBody
	parseParams             = cmdkit.ParseParams
	parsePayload            = cmdkit.ParsePayload
	printMutationResult     = cmdkit.PrintMutationResult
	printResult             = cmdkit.PrintResult
	requireForceOrDryRun    = cmdkit.RequireForceOrDryRun
	requireValue            = cmdkit.RequireValue
	resolveUpdateBody       = cmdkit.ResolveUpdateBody
	resultItems             = cmdkit.ResultItems
	sortedKeys              = cmdkit.SortedKeys
	stringValue             = cmdkit.StringValue
	stringsToAny            = cmdkit.StringsToAny
	uniqueCSV               = cmdkit.UniqueCSV
	updateCommand           = cmdkit.UpdateCommand
	withParam               = cmdkit.WithParam
)
