package sbcli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type loadedConfig struct {
	Endpoint  string
	Token     string
	ProjectID string
	Output    string
	Agent     bool
}

func configPath(env []string) string {
	if dir, ok := envGet(env, "XDG_CONFIG_HOME"); ok && strings.TrimSpace(dir) != "" {
		return filepath.Join(dir, "simplebase", "config.json")
	}
	home, ok := envGet(env, "HOME")
	if !ok || home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, ".config", "simplebase", "config.json")
}

func loadFileConfig(env []string) (loadedConfig, error) {
	path := configPath(env)
	st, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return loadedConfig{}, nil
		}
		return loadedConfig{}, err
	}
	perm := st.Mode().Perm()
	if perm&0o077 != 0 {
		return loadedConfig{}, errInsecureConfig
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return loadedConfig{}, err
	}
	var raw struct {
		Endpoint  string `json:"endpoint"`
		Token     string `json:"token"`
		ProjectID string `json:"project_id"`
		Output    string `json:"output"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return loadedConfig{}, err
	}
	return loadedConfig{Endpoint: raw.Endpoint, Token: raw.Token, ProjectID: raw.ProjectID, Output: raw.Output}, nil
}

var errInsecureConfig = errors.New("config_insecure")

func envGet(env []string, key string) (string, bool) {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return strings.TrimPrefix(env[i], prefix), true
		}
	}
	return "", false
}

func applyEnv(cfg loadedConfig, env []string) loadedConfig {
	if v, ok := envGet(env, "SIMPLEBASE_URL"); ok {
		cfg.Endpoint = v
	}
	if v, ok := envGet(env, "SIMPLEBASE_TOKEN"); ok {
		cfg.Token = v
	}
	if v, ok := envGet(env, "SIMPLEBASE_PROJECT_ID"); ok {
		cfg.ProjectID = v
	}
	if v, ok := envGet(env, "SIMPLEBASE_OUTPUT"); ok {
		cfg.Output = v
	}
	if v, ok := envGet(env, "SIMPLEBASE_AGENT_RUN"); ok && (v == "1" || strings.EqualFold(v, "true")) {
		cfg.Agent = true
	}
	return cfg
}
