package cmdkit

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"umbraco-cli/internal/api"
)

// MutationCandidate is one method+path attempt for a mutation whose route
// or HTTP method moved between Management API versions.
type MutationCandidate struct {
	Method string
	Path   string
}

// MutateWithFallback issues the mutation against each candidate in order,
// falling back past 404/405 so commands keep working on both modern and
// older Management API versions. Dry-run plans the first (modern)
// candidate. Note a 404 can also mean the target entity does not exist —
// in that case every candidate 404s and the last error is returned.
func MutateWithFallback(ctx context.Context, client *api.Client, body any, opts api.RequestOptions, candidates ...MutationCandidate) (any, error) {
	var lastErr error
	for i, candidate := range candidates {
		result, err := client.Request(ctx, candidate.Method, candidate.Path, body, opts)
		if err == nil {
			return result, nil
		}
		var apiErr *api.APIError
		retriable := errors.As(err, &apiErr) &&
			(apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusMethodNotAllowed)
		if retriable && i < len(candidates)-1 {
			lastErr = err
			continue
		}
		return nil, err
	}
	return nil, lastErr
}

// GetRequestCandidate is one path+options attempt for a read whose route
// moved between Management API versions.
type GetRequestCandidate struct {
	Path string
	Opts api.RequestOptions
}

// AutoPaginateDefaultPageSize is the page size used when --all is set
// without an explicit --take. 500 balances trip count vs. payload size and
// matches the practical chunk Umbraco's tree endpoints serve cleanly.
const AutoPaginateDefaultPageSize = 500

// autoPaginateMaxPages caps the number of pages walked per --all run.
// 500 items × 200 pages = 100k items hard ceiling — anything larger should
// use --skip/--take manually, both as a sanity check and because a single
// 100k-item JSON envelope is not something an agent should be paging into
// memory by accident.
const autoPaginateMaxPages = 200

// GetAllPagesWithFallback walks the paginated endpoint behind the candidate
// list and accumulates every item into a single {items, total} envelope.
// Used by commands that pass --all. After the first page resolves, only the
// winning candidate is queried — re-running the full fallback chain would
// re-issue the 404ing candidates once per page.
//
// pageSize ≤ 0 falls back to autoPaginateDefaultPageSize. baseSkip < 0
// is treated as 0. limit > 0 stops the loop once `limit` items have been
// accumulated (used to honour --first-n without pulling pages we'd discard).
func GetAllPagesWithFallback(
	ctx context.Context,
	client *api.Client,
	pageSize int,
	baseSkip int,
	limit int,
	candidates ...GetRequestCandidate,
) (any, error) {
	if pageSize <= 0 {
		pageSize = AutoPaginateDefaultPageSize
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
		paged := make([]GetRequestCandidate, len(candidates))
		for i, c := range candidates {
			params := map[string]any{}
			for k, v := range c.Opts.Params {
				params[k] = v
			}
			params["skip"] = skip
			params["take"] = pageSize
			opts := c.Opts
			opts.Params = params
			paged[i] = GetRequestCandidate{Path: c.Path, Opts: opts}
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

// GetWithFallback returns the first candidate's response that is not a 404
// (see getWithFallbackIndex).
func GetWithFallback(ctx context.Context, client *api.Client, candidates ...GetRequestCandidate) (any, error) {
	result, _, err := getWithFallbackIndex(ctx, client, candidates...)
	return result, err
}

// getWithFallbackIndex tries each candidate in order, skipping past 404s
// (endpoint not present on this Umbraco version) and returning the index of
// the candidate that answered so paged callers can stop re-probing.
func getWithFallbackIndex(ctx context.Context, client *api.Client, candidates ...GetRequestCandidate) (any, int, error) {
	var lastNotFound error

	for index, candidate := range candidates {
		result, err := client.Get(ctx, candidate.Path, candidate.Opts)
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
