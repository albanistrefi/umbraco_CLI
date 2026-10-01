package cmdkit

import (
	"reflect"
	"testing"

	"umbraco-cli/internal/config"
)

func TestDependenciesProviders(t *testing.T) {
	var deps Dependencies
	if deps.RequestedOutput() != "" || deps.CurrentEnvOutput() != "" || deps.CurrentConfig().BaseURL != "" {
		t.Fatal("expected zero Dependencies to read as empty")
	}
	if !reflect.DeepEqual(deps.ConfigOptions(), config.LoadOptions{}) {
		t.Fatal("expected zero config options")
	}
	output := "table"
	deps = Dependencies{
		OutputFlag:            &output,
		EnvOutput:             config.OutputPlain,
		Config:                config.Config{BaseURL: "https://static.test"},
		EnvOutputProvider:     func() config.OutputFormat { return config.OutputJSON },
		ConfigProvider:        func() config.Config { return config.Config{BaseURL: "https://runtime.test"} },
		ConfigOptionsProvider: func() config.LoadOptions { return config.LoadOptions{Profile: "p"} },
	}
	if deps.RequestedOutput() != "table" || deps.CurrentEnvOutput() != config.OutputJSON ||
		deps.CurrentConfig().BaseURL != "https://runtime.test" || deps.ConfigOptions().Profile != "p" {
		t.Fatalf("expected providers to win over static values, got %+v", deps)
	}
	deps.EnvOutputProvider = nil
	deps.ConfigProvider = nil
	if deps.CurrentEnvOutput() != config.OutputPlain || deps.CurrentConfig().BaseURL != "https://static.test" {
		t.Fatal("expected static values without providers")
	}
}
