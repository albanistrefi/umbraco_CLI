// Code in this package predates the cmdkit/cmdtest split and still uses the
// shared toolkit and test harness by their original unexported names. These
// aliases keep those call sites unchanged; add-on packages import cmdkit and
// cmdtest directly and never this package.

package commands

import (
	"umbraco-cli/internal/commands/cmdkit"
)

const (
	autoPaginateDefaultPageSize = cmdkit.AutoPaginateDefaultPageSize
	logViewerLogPath            = cmdkit.LogViewerLogPath
)

type (
	Dependencies        = cmdkit.Dependencies
	collectionSpec      = cmdkit.CollectionSpec
	createSpec          = cmdkit.CreateSpec
	deleteSpec          = cmdkit.DeleteSpec
	getRequestCandidate = cmdkit.GetRequestCandidate
	getSpec             = cmdkit.GetSpec
	mutationCandidate   = cmdkit.MutationCandidate
	readTriageOptions   = cmdkit.ReadTriageOptions
	searchSpec          = cmdkit.SearchSpec
	targetActionSpec    = cmdkit.TargetActionSpec
	updateSpec          = cmdkit.UpdateSpec
)

var (
	addAutoPaginationFlag   = cmdkit.AddAutoPaginationFlag
	addBackupFlag           = cmdkit.AddBackupFlag
	addDryRunFlag           = cmdkit.AddDryRunFlag
	addFieldsFlag           = cmdkit.AddFieldsFlag
	addPaginationFlags      = cmdkit.AddPaginationFlags
	addReadTriageFlags      = cmdkit.AddReadTriageFlags
	aliasObject             = cmdkit.AliasObject
	applyFieldsProjection   = cmdkit.ApplyFieldsProjection
	applyPaginationParams   = cmdkit.ApplyPaginationParams
	applyReadTriage         = cmdkit.ApplyReadTriage
	asString                = cmdkit.AsString
	cloneAliasValue         = cmdkit.CloneAliasValue
	cloneAnyMap             = cmdkit.CloneAnyMap
	cloneObject             = cmdkit.CloneObject
	collectionCommand       = cmdkit.CollectionCommand
	createCommand           = cmdkit.CreateCommand
	createResult            = cmdkit.CreateResult
	deleteCommand           = cmdkit.DeleteCommand
	ensurePayloadID         = cmdkit.EnsurePayloadID
	fetchObject             = cmdkit.FetchObject
	getAllPagesWithFallback = cmdkit.GetAllPagesWithFallback
	getCommand              = cmdkit.GetCommand
	getWithFallback         = cmdkit.GetWithFallback
	indexerHealthStatus     = cmdkit.IndexerHealthStatus
	isAPIStatus             = cmdkit.IsAPIStatus
	isUUIDLike              = cmdkit.IsUUIDLike
	itemID                  = cmdkit.ItemID
	jsonShapeName           = cmdkit.JSONShapeName
	mergeAliasPayload       = cmdkit.MergeAliasPayload
	mergeAliasValue         = cmdkit.MergeAliasValue
	mergeParams             = cmdkit.MergeParams
	newUUIDv4               = cmdkit.NewUUIDv4
	optionalBody            = cmdkit.OptionalBody
	parseJSONObject         = cmdkit.ParseJSONObject
	parseParams             = cmdkit.ParseParams
	parsePayload            = cmdkit.ParsePayload
	printMutationResult     = cmdkit.PrintMutationResult
	printResult             = cmdkit.PrintResult
	requireForceOrDryRun    = cmdkit.RequireForceOrDryRun
	requireValue            = cmdkit.RequireValue
	resolveBackupPath       = cmdkit.ResolveBackupPath
	resolveUpdateBody       = cmdkit.ResolveUpdateBody
	resultItems             = cmdkit.ResultItems
	searchCommand           = cmdkit.SearchCommand
	sortedKeys              = cmdkit.SortedKeys
	stringValue             = cmdkit.StringValue
	stringsToAny            = cmdkit.StringsToAny
	stripFields             = cmdkit.StripFields
	targetActionCommand     = cmdkit.TargetActionCommand
	uniqueCSV               = cmdkit.UniqueCSV
	updateCommand           = cmdkit.UpdateCommand
	withParam               = cmdkit.WithParam
	writeBackup             = cmdkit.WriteBackup
)
