package commands

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"umbraco-cli/internal/api"
)

type getRequestCandidate struct {
	path string
	opts api.RequestOptions
}

// autoPaginateDefaultPageSize is the page size used when --all is set
// without an explicit --take. 500 balances trip count vs. payload size and
// matches the practical chunk Umbraco's tree endpoints serve cleanly.
const autoPaginateDefaultPageSize = 500

// autoPaginateMaxPages caps the number of pages walked per --all run.
// 500 items × 200 pages = 100k items hard ceiling — anything larger should
// use --skip/--take manually, both as a sanity check and because a single
// 100k-item JSON envelope is not something an agent should be paging into
// memory by accident.
const autoPaginateMaxPages = 200

// getAllPagesWithFallback walks the paginated endpoint behind the candidate
// list and accumulates every item into a single {items, total} envelope.
// Used by commands that pass --all. After the first page resolves, only the
// winning candidate is queried — re-running the full fallback chain would
// re-issue the 404ing candidates once per page.
//
// pageSize ≤ 0 falls back to autoPaginateDefaultPageSize. baseSkip < 0
// is treated as 0. limit > 0 stops the loop once `limit` items have been
// accumulated (used to honour --first-n without pulling pages we'd discard).
func getAllPagesWithFallback(
	ctx context.Context,
	client *api.Client,
	pageSize int,
	baseSkip int,
	limit int,
	candidates ...getRequestCandidate,
) (any, error) {
	if pageSize <= 0 {
		pageSize = autoPaginateDefaultPageSize
	}
	if baseSkip < 0 {
		baseSkip = 0
	}

	var all []any
	var total any
	skip := baseSkip
	exhausted := false
	limitReached := false

	for iter := 0; iter < autoPaginateMaxPages; iter++ {
		paged := make([]getRequestCandidate, len(candidates))
		for i, c := range candidates {
			params := map[string]any{}
			for k, v := range c.opts.Params {
				params[k] = v
			}
			params["skip"] = skip
			params["take"] = pageSize
			opts := c.opts
			opts.Params = params
			paged[i] = getRequestCandidate{path: c.path, opts: opts}
		}

		result, winner, err := getWithFallbackIndex(ctx, client, paged...)
		if err != nil {
			return nil, err
		}
		if winner > 0 {
			candidates = candidates[winner:]
		}
		envelope, ok := result.(map[string]any)
		if !ok {
			// Endpoint didn't return the standard {items, total} shape —
			// return verbatim and let the caller deal with it.
			return result, nil
		}
		items, _ := envelope["items"].([]any)
		all = append(all, items...)
		if total == nil {
			total = envelope["total"]
		}

		if limit > 0 && len(all) >= limit {
			all = all[:limit]
			limitReached = true
			break
		}
		if len(items) < pageSize {
			exhausted = true
			break
		}
		skip += pageSize
	}

	// If neither exit condition fired the loop hit the safety ceiling
	// (autoPaginateMaxPages × pageSize items pulled, no short page seen).
	// Returning a normal envelope here would silently truncate large
	// collections — surface it as an error so callers don't mistake a cap
	// hit for a complete walk. --first-n early exits do NOT count as
	// truncation: the caller asked for at most N items and got them.
	if !exhausted && !limitReached {
		// skip already points at the next unread page (it's advanced at the
		// end of each iteration that didn't see a short page), so the resume
		// offset is `skip`, NOT `skip+pageSize` — adding pageSize would skip
		// the very next page of data the caller is trying to resume from.
		return nil, fmt.Errorf("--all hit the safety ceiling of %d pages × %d items = %d after %d items collected; the collection has more items than the auto-paginator will walk in one shot. Use --skip %d to resume from this offset, or --take with a larger page size to raise the ceiling",
			autoPaginateMaxPages, pageSize, autoPaginateMaxPages*pageSize, len(all), skip)
	}

	return map[string]any{"items": all, "total": total}, nil
}

func getWithFallback(ctx context.Context, client *api.Client, candidates ...getRequestCandidate) (any, error) {
	result, _, err := getWithFallbackIndex(ctx, client, candidates...)
	return result, err
}

// getWithFallbackIndex tries each candidate in order, skipping past 404s
// (endpoint not present on this Umbraco version) and returning the index of
// the candidate that answered so paged callers can stop re-probing.
func getWithFallbackIndex(ctx context.Context, client *api.Client, candidates ...getRequestCandidate) (any, int, error) {
	var lastNotFound error

	for index, candidate := range candidates {
		result, err := client.Get(ctx, candidate.path, candidate.opts)
		if err == nil {
			return result, index, nil
		}

		var apiErr *api.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			lastNotFound = err
			continue
		}

		return nil, 0, err
	}

	if lastNotFound != nil {
		return nil, 0, lastNotFound
	}

	return nil, 0, fmt.Errorf("no endpoint candidates were configured")
}
