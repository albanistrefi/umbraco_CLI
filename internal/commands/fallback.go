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
