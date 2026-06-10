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

// NewRuntime resolves config and wires the API client. A config resolution
// failure does not fail runtime construction — informational commands
// (--help, --version, schema, generate-skills) must keep working on a
// broken setup. The error is carried inside the client instead, so any
// command that actually reaches for the API reports the real cause.
func NewRuntime() *Runtime {
	httpClient := &http.Client{Timeout: 60 * time.Second}

	cfg, err := config.Load()
	if err != nil {
		return &Runtime{HTTPClient: httpClient, Client: api.NewUnavailableClient(err)}
	}

	tokenProvider := auth.New(cfg, httpClient)
	client := api.NewClient(cfg, httpClient, tokenProvider)
	return &Runtime{Config: cfg, Client: client, HTTPClient: httpClient}
}
