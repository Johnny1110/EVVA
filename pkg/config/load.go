package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/joho/godotenv"
)

// LoadOptions tunes Load. Zero-value fields fall back to LoadDefault
// behavior — AppName="evva", AppHome=~/.evva/, WorkDir=os.Getwd(),
// AppVersion=DefaultAppVersion. Downstream apps that want a different
// home dir or app name fill in the relevant fields.
type LoadOptions struct {
	AppName    string // brand identifier; drives the AppHome layout. Defaults to "evva".
	AppHome    string // absolute path; defaults to ~/.<AppName>/.
	WorkDir    string // process cwd; defaults to os.Getwd().
	AppVersion string // version string for diagnostics; defaults to DefaultAppVersion.

	// EnvAliases maps the caller's preferred env-var names onto evva's
	// canonical ones BEFORE godotenv.Load runs. Useful when a downstream
	// app advertises friendlier spellings — e.g. `{"LOGDIR": "LOG_DIR",
	// "LOGLEVEL": "LOG_LEVEL"}` lets a friday user write either form in
	// `~/.friday/.env` and have evva's loader pick it up.
	//
	// The promotion is non-overriding: an alias only seeds the canonical
	// name when that canonical name is unset. Existing canonical exports
	// win, so a deliberate `LOG_DIR=...` is never clobbered by a stray
	// alias.
	EnvAliases map[string]string

	// EnvOverrides runs AFTER the YAML + canonical env-vars have built
	// the Config. Each function gets the populated *Config and can fold
	// in env vars that don't have a native hook inside Load (e.g.
	// MAX_ITERS → cfg.SetMaxIterations). The first error short-circuits
	// the rest and is returned from Load.
	//
	// Use this to translate downstream-flavoured env conventions
	// (APIKEY → cfg.SetProviderCredentials, MAX_ITERS → cfg.SetMaxIterations)
	// in one place instead of post-Load shim code at every call site.
	EnvOverrides []func(*Config) error
}

// LoadDefault returns a Config populated with evva's historical defaults:
// AppName="evva", AppHome=~/.evva/, WorkDir=os.Getwd(). Intended for the
// bundled cmd/evva binary and for backward-compatible callers.
//
// Startup failures (missing/invalid YAML, unknown provider/model) bail
// with os.Exit so the user gets a clear single-line error rather than a
// panic stack from deep inside the agent boot path.
func LoadDefault() *Config {
	cfg, err := Load(LoadOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "evva: %v\n", err)
		os.Exit(1)
	}
	return cfg
}

// Load parses env vars + the per-user YAML and returns a populated Config.
// Each LoadOptions field has a sensible default (see LoadOptions doc).
//
// Unlike LoadDefault, Load returns an error instead of calling os.Exit so
// downstream hosts can surface it through their own error path.
func Load(opts LoadOptions) (*Config, error) {
	appName := opts.AppName
	if appName == "" {
		appName = DefaultAppName
	}
	appVersion := opts.AppVersion
	if appVersion == "" {
		appVersion = Version
		if appVersion == "" {
			appVersion = DefaultAppVersion
		}
	}
	appHome := opts.AppHome
	if appHome == "" {
		homeDir, _ := os.UserHomeDir()
		if runtime.GOOS == "windows" {
			appHome = homeDir + `\.` + appName
		} else {
			appHome = homeDir + "/." + appName
		}
	}
	workdir := opts.WorkDir
	if workdir == "" {
		wd, err := os.Getwd()
		if err != nil {
			wd = "."
		}
		workdir = wd
	}

	// Promote any caller-declared env-var aliases into evva's canonical
	// names BEFORE godotenv reads .env. Non-overriding: an existing
	// canonical export wins.
	applyEnvAliases(opts.EnvAliases)

	// load deployment-level vars from .env (logging, app env, dir overrides)
	godotenv.Load(appHome + "/.env")

	// Re-apply aliases after godotenv runs so a .env file using the alias
	// form (e.g. `LOGDIR=/var/log/friday`) also promotes into the
	// canonical name. godotenv.Load is non-overriding, so this two-pass
	// approach lets the alias work whether the user exports it in the
	// shell or writes it in .env.
	applyEnvAliases(opts.EnvAliases)

	cfgPath := filepath.Join(appHome, "config", appName+"-config.yml")
	fileCfg, created, err := LoadFileConfig(cfgPath, appName)
	if err != nil {
		return nil, err
	}
	if created {
		fmt.Fprintf(os.Stderr,
			"%s: wrote new config to %s — fill in your API keys to use cloud providers.\n",
			appName, cfgPath)
	}

	defProvider, defModel, err := ResolveDefaultModel(fileCfg.DefaultProvider, fileCfg.DefaultModel)
	if err != nil {
		return nil, err
	}

	enableAutoMem := true
	if fileCfg.EnableAutoMemory != nil {
		enableAutoMem = *fileCfg.EnableAutoMemory
	}
	// Env override: EVVA_AUTO_MEMORY=0/false forces off regardless of YAML.
	if v := os.Getenv("EVVA_AUTO_MEMORY"); v != "" {
		switch v {
		case "0", "false", "FALSE", "off", "OFF", "no", "NO":
			enableAutoMem = false
		case "1", "true", "TRUE", "on", "ON", "yes", "YES":
			enableAutoMem = true
		}
	}

	cfg := &Config{
		AppName:    appName,
		AppVersion: appVersion,
		OS:         runtime.GOOS,
		AppEnv:     getEnvDefaultLowerCase("APP_ENV", "dev"),

		// log
		LogLevel:  getEnvDefaultLowerCase("LOG_LEVEL", "info"),
		LogFormat: getEnvDefaultLowerCase("LOG_FORMAT", "text"),
		LogDir:    resolveLogDir(appHome),

		// per-user home dir
		AppHome:            appHome,
		AppHomeSkillsDir:   appHome + "/" + getEnvDefault("SKILLS_DIR", "skills"),
		AppHomeUserProfile: appHome + "/" + getEnvDefault("USER_PROFILE", "user_profile.md"),
		AppHomeConfigFile:  cfgPath,

		// from YAML
		DefaultMaxIterations: fileCfg.MaxIterations,
		DefaultMaxTokens:     fileCfg.MaxTokens,
		AutoCompactThreshold: fileCfg.AutoCompactThreshold,
		DisplayThinking:      fileCfg.DisplayThinking,
		EnableAutoMemory:     enableAutoMem,
		TavilyAPIKey:         fileCfg.TavilyAPIKey,
		FetchMaxBytes:        fileCfg.FetchMaxBytes,
		DefaultProvider:      defProvider,
		DefaultModel:         defModel,
		DefaultEffort:        fileCfg.DefaultEffort,
		DefaultProfile:       fileCfg.DefaultProfile,
		PermissionMode:       fileCfg.PermissionMode,

		LoadedAt: time.Now(),
	}

	setupGlobalParam(cfg)
	setupWorkDirParam(cfg, workdir)
	setupLLMProviderConfig(cfg, fileCfg)

	// Apply caller-declared env overrides last so they can mutate the
	// already-populated cfg (e.g. fold MAX_ITERS into DefaultMaxIterations
	// without a post-Load shim). Short-circuits on first error.
	for _, fn := range opts.EnvOverrides {
		if fn == nil {
			continue
		}
		if err := fn(cfg); err != nil {
			return nil, fmt.Errorf("config: EnvOverrides: %w", err)
		}
	}

	return cfg, nil
}

// applyEnvAliases promotes the values of alias env vars into the
// canonical names listed in m. Non-overriding: an existing canonical
// export is never clobbered. Empty values are skipped.
func applyEnvAliases(m map[string]string) {
	for alias, canonical := range m {
		if alias == "" || canonical == "" {
			continue
		}
		v := os.Getenv(alias)
		if v == "" {
			continue
		}
		if os.Getenv(canonical) != "" {
			continue
		}
		_ = os.Setenv(canonical, v)
	}
}
