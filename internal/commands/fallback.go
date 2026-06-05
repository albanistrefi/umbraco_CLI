package commands

import (
	"context"
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
// Used by commands that pass --all. Each iteration re-runs getWithFallback,
// which is fine because the first successful candidate remains successful
// on subsequent pages — no extra round-trips in steady state.
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

		result, err := getWithFallback(ctx, client, paged...)
		if err != nil {
			return nil, err
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
			break
		}
		if len(items) < pageSize {
			break
		}
		skip += pageSize
	}

	return map[string]any{"items": all, "total": total}, nil
}

func getWithFallback(ctx context.Context, client *api.Client, candidates ...getRequestCandidate) (any, error) {
	var lastNotFound error

	for _, candidate := range candidates {
		result, err := client.Get(ctx, candidate.path, candidate.opts)
		if err == nil {
			return result, nil
		}

		apiErr, ok := err.(*api.APIError)
		if ok && apiErr.StatusCode == http.StatusNotFound {
			lastNotFound = err
			continue
		}

		return nil, err
	}

	if lastNotFound != nil {
		return nil, lastNotFound
	}

	return nil, fmt.Errorf("no endpoint candidates were configured")
}
