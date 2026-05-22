package config_test

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/johnny1110/evva/pkg/config"
)

// ExampleLoad demonstrates the canonical downstream-app load: pick an
// AppName + AppHome, let Load auto-create the YAML on first run, and
// trust the returned *Config from then on.
//
// AppName drives the AppHome layout (~/.{AppName}/) AND the first-run
// YAML's `default_profile` value — running this with AppName="friday"
// stamps `default_profile: friday`, not "evva" (Phase 19b).
func ExampleLoad() {
	tmp, _ := filepath.Abs("/tmp/evva-example-load")

	cfg, err := config.Load(config.LoadOptions{
		AppName: "friday",
		AppHome: tmp,
		WorkDir: tmp + "/work",
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("app:", cfg.AppName)
	fmt.Println("default_profile:", cfg.DefaultProfile)
	fmt.Println("config_file_endswith:", strings.HasSuffix(cfg.AppHomeConfigFile, "friday-config.yml"))
	// Output:
	// app: friday
	// default_profile: friday
	// config_file_endswith: true
}

// ExampleConfig_SetProviderCredentials shows the Phase 19b thread-safe
// setter for LLM credentials. Prefer this over direct map assignment
// when wiring providers at runtime — direct writes race concurrent
// reads on the same *Config.
func ExampleConfig_SetProviderCredentials() {
	tmp, _ := filepath.Abs("/tmp/evva-example-creds")
	cfg, _ := config.Load(config.LoadOptions{
		AppName: "alpha", AppHome: tmp, WorkDir: tmp,
	})

	if err := cfg.SetProviderCredentials(
		"deepseek",
		"https://api.deepseek.com",
		"sk-example-key",
	); err != nil {
		fmt.Println("error:", err)
		return
	}

	got := cfg.LLMProviderConfig["deepseek"]
	fmt.Println("api_url:", got.ApiURL)
	fmt.Println("api_secret_present:", got.ApiSecret != "")
	// Output:
	// api_url: https://api.deepseek.com
	// api_secret_present: true
}
