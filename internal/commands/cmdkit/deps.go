package cmdkit

import (
	"net/http"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/config"
)

type Dependencies struct {
	Client                *api.Client
	Config                config.Config
	HTTPClient            *http.Client
	EnvOutput             config.OutputFormat
	OutputFlag            *string
	EnvOutputProvider     func() config.OutputFormat
	ConfigOptionsProvider func() config.LoadOptions
	// ConfigProvider returns the resolved config AFTER --profile/--config
	// selection: the Config value above is copied at command-tree
	// construction, before PersistentPreRunE reloads the runtime, so
	// commands that need the base URL must read it through this provider
	// or they will target the default environment regardless of --profile.
	ConfigProvider func() config.Config
}

func (d Dependencies) RequestedOutput() string {
	if d.OutputFlag == nil {
		return ""
	}
	return *d.OutputFlag
}

func (d Dependencies) CurrentConfig() config.Config {
	if d.ConfigProvider != nil {
		return d.ConfigProvider()
	}
	return d.Config
}

func (d Dependencies) CurrentEnvOutput() config.OutputFormat {
	if d.EnvOutputProvider != nil {
		return d.EnvOutputProvider()
	}
	return d.EnvOutput
}

func (d Dependencies) ConfigOptions() config.LoadOptions {
	if d.ConfigOptionsProvider != nil {
		return d.ConfigOptionsProvider()
	}
	return config.LoadOptions{}
}
