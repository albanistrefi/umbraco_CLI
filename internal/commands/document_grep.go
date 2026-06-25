package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/api"
)

const defaultDocumentGrepConcurrency = 6

type documentGrepOptions struct {
	Needle      string
	Regex       bool
	IgnoreCase  bool
	Published   bool
	Draft       bool
	Properties  []string
	Doctypes    []string
	StartID     string
	Concurrency int
}

type documentGrepResult struct {
	Query            string                `json:"query"`
	Regex            bool                  `json:"regex"`
	IgnoreCase       bool                  `json:"ignoreCase"`
	Mode             string                `json:"mode"`
	StartID          string                `json:"startId,omitempty"`
	DocumentsWalked  int                   `json:"documentsWalked"`
	DocumentsFetched int                   `json:"documentsFetched"`
	DocumentsMatched int                   `json:"documentsMatched"`
	Hits             []documentGrepHit     `json:"hits"`
	Skipped          []documentGrepSkipped `json:"skipped,omitempty"`
}

type documentGrepHit struct {
	DocumentID        string `json:"documentId"`
	DocumentName      string `json:"documentName,omitempty"`
	DocumentTypeAlias string `json:"documentTypeAlias,omitempty"`
	State             string `json:"state"`
	PropertyAlias     string `json:"propertyAlias"`
	Match             string `json:"match"`
	Snippet           string `json:"snippet"`
	MatchIndex        int    `json:"matchIndex"`
}

type documentGrepSkipped struct {
	ID    string `json:"id"`
	Stage string `json:"stage"`
	Error string `json:"error"`
}

type documentGrepMatcher struct {
	needle string
	regex  *regexp.Regexp
}

func documentGrep(deps Dependencies) *cobra.Command {
	opts := documentGrepOptions{Concurrency: defaultDocumentGrepConcurrency}
	cmd := &cobra.Command{
		Use:   "grep <substring>",
		Short: "Exhaustively scan document property values for an exact substring",
		Long: `Walks the document tree, fetches each document, and scans each serialized
property value for an exact substring. This is intentionally different from
document search: search is Examine-backed and can miss buried URLs, aliases,
tokens, or strings inside rich text and block/grid JSON.

By default grep scans both the current draft representation and the published
snapshot when one exists. Use --draft or --published to narrow the scan.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Needle = args[0]
			result, err := executeDocumentGrep(cmd.Context(), cmd, deps, opts)
			if err != nil {
				return err
			}
			return printResult(cmd, deps, result)
		},
	}
	cmd.Flags().BoolVar(&opts.Regex, "regex", false, "Treat the substring argument as a regular expression")
	cmd.Flags().BoolVarP(&opts.IgnoreCase, "ignore-case", "i", false, "Match case-insensitively")
	cmd.Flags().BoolVar(&opts.Published, "published", false, "Scan only published document snapshots")
	cmd.Flags().BoolVar(&opts.Draft, "draft", false, "Scan only current draft document payloads")
	cmd.Flags().StringArrayVar(&opts.Properties, "property", nil, "Restrict matches to a property alias; repeat for multiple aliases")
	cmd.Flags().StringArrayVar(&opts.Doctypes, "doctype", nil, "Restrict matches to a document type alias; repeat for multiple aliases")
	cmd.Flags().StringVar(&opts.StartID, "start-id", "", "Document ID whose subtree should be scanned instead of the full tree")
	cmd.Flags().IntVar(&opts.Concurrency, "concurrency", defaultDocumentGrepConcurrency, "Maximum concurrent document fetches")
	return cmd
}

func executeDocumentGrep(ctx context.Context, cmd *cobra.Command, deps Dependencies, opts documentGrepOptions) (documentGrepResult, error) {
	if opts.Needle == "" {
		return documentGrepResult{}, fmt.Errorf("document grep requires a non-empty substring")
	}
	if opts.Published && opts.Draft {
		return documentGrepResult{}, fmt.Errorf("use either --published or --draft, not both")
	}
	if opts.Concurrency <= 0 {
		return documentGrepResult{}, fmt.Errorf("--concurrency must be greater than zero")
	}

	matcher, err := newDocumentGrepMatcher(opts)
	if err != nil {
		return documentGrepResult{}, err
	}

	result := documentGrepResult{
		Query:      opts.Needle,
		Regex:      opts.Regex,
		IgnoreCase: opts.IgnoreCase,
		Mode:       documentGrepMode(opts),
		StartID:    opts.StartID,
		Hits:       []documentGrepHit{},
	}
	propertyFilter := stringSet(opts.Properties)
	doctypeFilter := stringSet(opts.Doctypes)

	ids := make(chan string)
	var mu sync.Mutex
	matchedDocs := map[string]struct{}{}
	doctypeCache := map[string]string{}
	var workers sync.WaitGroup

	for i := 0; i < opts.Concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for id := range ids {
				fetched, hits, skipped := grepOneDocument(ctx, deps.Client, id, opts, matcher, propertyFilter, doctypeFilter, doctypeCache, &mu)
				mu.Lock()
				result.DocumentsFetched += fetched
				result.Hits = append(result.Hits, hits...)
				result.Skipped = append(result.Skipped, skipped...)
				if len(hits) > 0 {
					matchedDocs[id] = struct{}{}
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "document grep: walked=%d fetched=%d hits=%d skipped=%d\n", result.DocumentsWalked, result.DocumentsFetched, len(result.Hits), len(result.Skipped))
				mu.Unlock()
			}
		}()
	}

	walkErr := walkDocumentTree(ctx, deps.Client, opts.StartID, func(id string) {
		mu.Lock()
		result.DocumentsWalked++
		mu.Unlock()
		ids <- id
	}, func(skipped documentGrepSkipped) {
		mu.Lock()
		result.Skipped = append(result.Skipped, skipped)
		mu.Unlock()
	})
	close(ids)
	workers.Wait()
	if walkErr != nil {
		return documentGrepResult{}, walkErr
	}

	sort.Slice(result.Hits, func(i, j int) bool {
		a, b := result.Hits[i], result.Hits[j]
		return strings.Join([]string{a.DocumentID, a.State, a.PropertyAlias, fmt.Sprintf("%08d", a.MatchIndex), a.Snippet}, "\x00") <
			strings.Join([]string{b.DocumentID, b.State, b.PropertyAlias, fmt.Sprintf("%08d", b.MatchIndex), b.Snippet}, "\x00")
	})
	sort.Slice(result.Skipped, func(i, j int) bool {
		a, b := result.Skipped[i], result.Skipped[j]
		return strings.Join([]string{a.ID, a.Stage, a.Error}, "\x00") < strings.Join([]string{b.ID, b.Stage, b.Error}, "\x00")
	})
	result.DocumentsMatched = len(matchedDocs)
	return result, nil
}

func grepOneDocument(ctx context.Context, client *api.Client, id string, opts documentGrepOptions, matcher documentGrepMatcher, propertyFilter map[string]struct{}, doctypeFilter map[string]struct{}, doctypeCache map[string]string, mu *sync.Mutex) (int, []documentGrepHit, []documentGrepSkipped) {
	var fetched int
	var hits []documentGrepHit
	var skipped []documentGrepSkipped
	for _, state := range documentGrepStates(opts) {
		doc, err := fetchDocumentForGrep(ctx, client, id, state)
		if err != nil {
			if state == "published" && !opts.Published && isAPIStatus(err, http.StatusNotFound) {
				continue
			}
			skipped = append(skipped, documentGrepSkipped{ID: id, Stage: state, Error: err.Error()})
			continue
		}
		fetched++
		alias := documentGrepDocumentTypeAlias(ctx, client, doc, doctypeCache, mu)
		if len(doctypeFilter) > 0 {
			if _, ok := doctypeFilter[alias]; !ok {
				continue
			}
		}
		hits = append(hits, scanDocumentPropertiesForGrep(doc, state, alias, matcher, propertyFilter)...)
	}
	return fetched, hits, skipped
}

func fetchDocumentForGrep(ctx context.Context, client *api.Client, id string, state string) (map[string]any, error) {
	path := api.JoinPath("/document/%s", id)
	if state == "published" {
		path = api.JoinPath("/document/%s/published", id)
	}
	return fetchObject(ctx, client, path, api.RequestOptions{})
}

func scanDocumentPropertiesForGrep(doc map[string]any, state string, doctypeAlias string, matcher documentGrepMatcher, propertyFilter map[string]struct{}) []documentGrepHit {
	id, _ := doc["id"].(string)
	name := documentGrepDocumentName(doc)
	values, _ := doc["values"].([]any)
	var hits []documentGrepHit
	for _, raw := range values {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		alias, _ := entry["alias"].(string)
		if alias == "" {
			continue
		}
		if len(propertyFilter) > 0 {
			if _, ok := propertyFilter[alias]; !ok {
				continue
			}
		}
		text := serializedGrepValue(entry["value"])
		for _, match := range matcher.find(text) {
			hits = append(hits, documentGrepHit{
				DocumentID:        id,
				DocumentName:      name,
				DocumentTypeAlias: doctypeAlias,
				State:             state,
				PropertyAlias:     alias,
				Match:             text[match[0]:match[1]],
				Snippet:           grepSnippet(text, match[0], match[1]),
				MatchIndex:        match[0],
			})
		}
	}
	return hits
}

func walkDocumentTree(ctx context.Context, client *api.Client, startID string, visit func(string), skip func(documentGrepSkipped)) error {
	if strings.TrimSpace(startID) == "" {
		items, err := fetchDocumentTreeItems(ctx, client, "", true)
		if err != nil {
			return err
		}
		for _, item := range items {
			walkDocumentTreeItem(ctx, client, item, visit, skip)
		}
		return nil
	}
	visit(startID)
	children, err := fetchDocumentTreeItems(ctx, client, startID, false)
	if err != nil {
		if isAPIStatus(err, http.StatusNotFound) {
			return nil
		}
		skip(documentGrepSkipped{ID: startID, Stage: "children", Error: err.Error()})
		return nil
	}
	for _, item := range children {
		walkDocumentTreeItem(ctx, client, item, visit, skip)
	}
	return nil
}

func walkDocumentTreeItem(ctx context.Context, client *api.Client, item any, visit func(string), skip func(documentGrepSkipped)) {
	entry, ok := item.(map[string]any)
	if !ok {
		return
	}
	id, _ := entry["id"].(string)
	if id == "" {
		return
	}
	visit(id)
	children, err := fetchDocumentTreeItems(ctx, client, id, false)
	if err != nil {
		if isAPIStatus(err, http.StatusNotFound) {
			return
		}
		skip(documentGrepSkipped{ID: id, Stage: "children", Error: err.Error()})
		return
	}
	for _, child := range children {
		walkDocumentTreeItem(ctx, client, child, visit, skip)
	}
}

func fetchDocumentTreeItems(ctx context.Context, client *api.Client, parentID string, root bool) ([]any, error) {
	var candidates []getRequestCandidate
	if root {
		candidates = []getRequestCandidate{
			{path: "/tree/document/root"},
			{path: "/document/root"},
		}
	} else {
		candidates = []getRequestCandidate{
			{path: "/tree/document/children", opts: api.RequestOptions{Params: map[string]any{"parentId": parentID}}},
			{path: api.JoinPath("/document/%s/children", parentID)},
		}
	}
	result, err := getAllPagesWithFallback(ctx, client, autoPaginateDefaultPageSize, 0, 0, candidates...)
	if err != nil {
		return nil, err
	}
	envelope, ok := result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("document tree response did not include an item envelope")
	}
	items, _ := envelope["items"].([]any)
	return items, nil
}

func documentGrepDocumentTypeAlias(ctx context.Context, client *api.Client, doc map[string]any, cache map[string]string, mu *sync.Mutex) string {
	docType, _ := doc["documentType"].(map[string]any)
	if docType == nil {
		return ""
	}
	if alias, _ := docType["alias"].(string); alias != "" {
		return alias
	}
	id, _ := docType["id"].(string)
	if id == "" {
		return ""
	}
	mu.Lock()
	alias, known := cache[id]
	mu.Unlock()
	if known {
		return alias
	}
	detail, err := fetchObject(ctx, client, api.JoinPath("/document-type/%s", id), api.RequestOptions{})
	if err == nil {
		alias, _ = detail["alias"].(string)
	}
	mu.Lock()
	cache[id] = alias
	mu.Unlock()
	return alias
}

func documentGrepDocumentName(doc map[string]any) string {
	if name, _ := doc["name"].(string); name != "" {
		return name
	}
	for _, name := range treeItemNames(doc) {
		if name != "" {
			return name
		}
	}
	return ""
}

func newDocumentGrepMatcher(opts documentGrepOptions) (documentGrepMatcher, error) {
	matcher := documentGrepMatcher{needle: opts.Needle}
	if opts.Regex {
		pattern := opts.Needle
		if opts.IgnoreCase {
			pattern = "(?i:" + pattern + ")"
		}
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return documentGrepMatcher{}, fmt.Errorf("invalid --regex pattern: %w", err)
		}
		matcher.regex = compiled
		return matcher, nil
	}
	if opts.IgnoreCase {
		compiled, err := regexp.Compile("(?i:" + regexp.QuoteMeta(opts.Needle) + ")")
		if err != nil {
			return documentGrepMatcher{}, fmt.Errorf("invalid case-insensitive substring: %w", err)
		}
		matcher.regex = compiled
	}
	return matcher, nil
}

func (m documentGrepMatcher) find(text string) [][2]int {
	if m.regex != nil {
		indexes := m.regex.FindAllStringIndex(text, -1)
		matches := make([][2]int, 0, len(indexes))
		for _, index := range indexes {
			matches = append(matches, [2]int{index[0], index[1]})
		}
		return matches
	}
	var matches [][2]int
	offset := 0
	for {
		index := strings.Index(text[offset:], m.needle)
		if index < 0 {
			break
		}
		start := offset + index
		end := start + len(m.needle)
		matches = append(matches, [2]int{start, end})
		if end == offset {
			offset++
		} else {
			offset = end
		}
		if offset >= len(text) {
			break
		}
	}
	return matches
}

func serializedGrepValue(value any) string {
	switch typed := value.(type) {
	case string:
		if parsed, ok := parseEmbeddedJSON(typed); ok {
			if encoded, err := json.Marshal(parsed); err == nil {
				return encodedString(typed) + "\n" + string(encoded)
			}
		}
		return typed
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(encoded)
	}
}

func parseEmbeddedJSON(raw string) (any, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || (!strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[")) {
		return nil, false
	}
	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return nil, false
	}
	return parsed, true
}

func encodedString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return value
	}
	return string(encoded)
}

func grepSnippet(text string, start int, end int) string {
	const contextSize = 40
	left := start - contextSize
	if left < 0 {
		left = 0
	}
	left = nearestUTF8StartBefore(text, left)
	right := end + contextSize
	if right > len(text) {
		right = len(text)
	}
	right = nearestUTF8BoundaryAfter(text, right)
	snippet := strings.ReplaceAll(text[left:right], "\n", " ")
	if left > 0 {
		snippet = "..." + snippet
	}
	if right < len(text) {
		snippet += "..."
	}
	return snippet
}

func nearestUTF8StartBefore(text string, index int) int {
	if index <= 0 {
		return 0
	}
	if index >= len(text) {
		return len(text)
	}
	for index > 0 && !utf8.RuneStart(text[index]) {
		index--
	}
	return index
}

func nearestUTF8BoundaryAfter(text string, index int) int {
	if index <= 0 {
		return 0
	}
	if index >= len(text) {
		return len(text)
	}
	for index < len(text) && !utf8.RuneStart(text[index]) {
		index++
	}
	return index
}

func documentGrepMode(opts documentGrepOptions) string {
	if opts.Published {
		return "published"
	}
	if opts.Draft {
		return "draft"
	}
	return "both"
}

func documentGrepStates(opts documentGrepOptions) []string {
	switch {
	case opts.Published:
		return []string{"published"}
	case opts.Draft:
		return []string{"draft"}
	default:
		return []string{"draft", "published"}
	}
}

func stringSet(values []string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result[value] = struct{}{}
	}
	return result
}

func isAPIStatus(err error, status int) bool {
	var apiErr *api.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == status
}
