// Code in this package predates the cmdkit split and still calls the shared
// toolkit by its original unexported names. These aliases keep those call
// sites unchanged; add-on packages import cmdkit directly and never this
// package.

package commands

import (
	"umbraco-cli/internal/commands/cmdkit"
)

const (
	autoPaginateDefaultPageSize = cmdkit.AutoPaginateDefaultPageSize
	backupAutoValue             = cmdkit.BackupAutoValue
	logViewerLogPath            = cmdkit.LogViewerLogPath
)

type (
	Dependencies        = cmdkit.Dependencies
	backupBinary        = cmdkit.BackupBinary
	collectionSpec      = cmdkit.CollectionSpec
	createSpec          = cmdkit.CreateSpec
	deleteSpec          = cmdkit.DeleteSpec
	getRequestCandidate = cmdkit.GetRequestCandidate
	getSpec             = cmdkit.GetSpec
	mutationCandidate   = cmdkit.MutationCandidate
	outputTrimOptions   = cmdkit.OutputTrimOptions
	paramFlag           = cmdkit.ParamFlag
	readTriageOptions   = cmdkit.ReadTriageOptions
	referencesSpec      = cmdkit.ReferencesSpec
	searchSpec          = cmdkit.SearchSpec
	targetActionSpec    = cmdkit.TargetActionSpec
	updateSpec          = cmdkit.UpdateSpec
)

var (
	addAutoPaginationFlag      = cmdkit.AddAutoPaginationFlag
	addBackupFlag              = cmdkit.AddBackupFlag
	addDocumentOutputTrimFlags = cmdkit.AddDocumentOutputTrimFlags
	addDryRunFlag              = cmdkit.AddDryRunFlag
	addFieldsFlag              = cmdkit.AddFieldsFlag
	addPaginationFlags         = cmdkit.AddPaginationFlags
	addReadTriageFlags         = cmdkit.AddReadTriageFlags
	aliasObject                = cmdkit.AliasObject
	applyDocumentOutputTrim    = cmdkit.ApplyDocumentOutputTrim
	applyFieldsProjection      = cmdkit.ApplyFieldsProjection
	applyPaginationParams      = cmdkit.ApplyPaginationParams
	applyReadTriage            = cmdkit.ApplyReadTriage
	areReferencedCommand       = cmdkit.AreReferencedCommand
	asString                   = cmdkit.AsString
	backupBinaryPath           = cmdkit.BackupBinaryPath
	cloneAliasValue            = cmdkit.CloneAliasValue
	cloneAnyMap                = cmdkit.CloneAnyMap
	cloneObject                = cmdkit.CloneObject
	collectionCommand          = cmdkit.CollectionCommand
	createCommand              = cmdkit.CreateCommand
	createResult               = cmdkit.CreateResult
	cultureValue               = cmdkit.CultureValue
	deleteCommand              = cmdkit.DeleteCommand
	ensurePayloadID            = cmdkit.EnsurePayloadID
	fetchObject                = cmdkit.FetchObject
	getAllPagesWithFallback    = cmdkit.GetAllPagesWithFallback
	getCommand                 = cmdkit.GetCommand
	getWithFallback            = cmdkit.GetWithFallback
	indexerHealthStatus        = cmdkit.IndexerHealthStatus
	isAPIStatus                = cmdkit.IsAPIStatus
	isUUIDLike                 = cmdkit.IsUUIDLike
	itemID                     = cmdkit.ItemID
	jsonShapeName              = cmdkit.JSONShapeName
	mergeAliasPayload          = cmdkit.MergeAliasPayload
	mergeAliasValue            = cmdkit.MergeAliasValue
	mergeParams                = cmdkit.MergeParams
	mutateWithFallback         = cmdkit.MutateWithFallback
	newUUIDv4                  = cmdkit.NewUUIDv4
	objectFromResult           = cmdkit.ObjectFromResult
	optionalBody               = cmdkit.OptionalBody
	parseJSONObject            = cmdkit.ParseJSONObject
	parseParams                = cmdkit.ParseParams
	parsePayload               = cmdkit.ParsePayload
	printMutationResult        = cmdkit.PrintMutationResult
	printResult                = cmdkit.PrintResult
	readBackup                 = cmdkit.ReadBackup
	referencesCommand          = cmdkit.ReferencesCommand
	requireForceOrDryRun       = cmdkit.RequireForceOrDryRun
	requireValue               = cmdkit.RequireValue
	resolveBackupPath          = cmdkit.ResolveBackupPath
	resolveUpdateBody          = cmdkit.ResolveUpdateBody
	resultItems                = cmdkit.ResultItems
	safeChildPath              = cmdkit.SafeChildPath
	sanitizeFileName           = cmdkit.SanitizeFileName
	searchCommand              = cmdkit.SearchCommand
	sortedKeys                 = cmdkit.SortedKeys
	stringValue                = cmdkit.StringValue
	stringsToAny               = cmdkit.StringsToAny
	stripFields                = cmdkit.StripFields
	targetActionBody           = cmdkit.TargetActionBody
	targetActionCommand        = cmdkit.TargetActionCommand
	treeItemNames              = cmdkit.TreeItemNames
	uniqueCSV                  = cmdkit.UniqueCSV
	updateCommand              = cmdkit.UpdateCommand
	validateDocumentOutputTrim = cmdkit.ValidateDocumentOutputTrim
	withParam                  = cmdkit.WithParam
	writeBackup                = cmdkit.WriteBackup
)
