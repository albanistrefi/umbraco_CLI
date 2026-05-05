package commands

import (
	"context"
	"fmt"

	"umbraco-cli/internal/api"
)

func fetchDoctypeObject(ctx context.Context, client *api.Client, id string) (map[string]any, error) {
	result, err := client.Get(ctx, fmt.Sprintf("/document-type/%s", id), api.RequestOptions{})
	if err != nil {
		return nil, err
	}

	return decodeResult[map[string]any](result)
}
