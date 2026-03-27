package cli

import (
	"net/http"
	"time"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/auth"
	"umbraco-cli/internal/config"
)

type Runtime struct {
	Config     config.Config
	Client     *api.Client
	HTTPClient *http.Client
}

func NewRuntime() (*Runtime, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{Timeout: 60 * time.Second}
	tokenProvider := auth.New(cfg, httpClient)
	client := api.NewClient(cfg, httpClient, tokenProvider)

	return &Runtime{Config: cfg, Client: client, HTTPClient: httpClient}, nil
}
