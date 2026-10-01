package cmdkit

import (
	"net/http"

	"umbraco-cli/internal/api"
	"umbraco-cli/internal/config"
)

// Dependencies is what every command builder receives: the API client, the
// resolved configuration and output settings. internal/cli builds the
// production value; cmdtest builds fakes.
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

// RequestedOutput is the --output value, or "" when none was given.
func (d Dependencies) RequestedOutput() string {
	if d.OutputFlag == nil {
		return ""
	}
	return *d.OutputFlag
}

// CurrentConfig is the configuration resolved after --profile/--config.
func (d Dependencies) CurrentConfig() config.Config {
	if d.ConfigProvider != nil {
		return d.ConfigProvider()
	}
	return d.Config
}

// CurrentEnvOutput is the configured default output format.
func (d Dependencies) CurrentEnvOutput() config.OutputFormat {
	if d.EnvOutputProvider != nil {
		return d.EnvOutputProvider()
	}
	return d.EnvOutput
}

// ConfigOptions are the --profile/--config/--base-url selections.
func (d Dependencies) ConfigOptions() config.LoadOptions {
	if d.ConfigOptionsProvider != nil {
		return d.ConfigOptionsProvider()
	}
	return config.LoadOptions{}
}
