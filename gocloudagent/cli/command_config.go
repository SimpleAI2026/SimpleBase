// 文件说明：command_config.go — `config` 子命令：show / get / set / path / test，及键表

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// configKeys 是 config get/set 支持的键。
var configKeys = []struct {
	Name string
	Get  func(Config) string
	Set  func(*Config, string)
}{
	{"endpoint", func(c Config) string { return c.Endpoint }, func(c *Config, v string) { c.Endpoint = v }},
	{"token", func(c Config) string { return c.Token }, func(c *Config, v string) { c.Token = v }},
	{"model", func(c Config) string { return c.Model }, func(c *Config, v string) { c.Model = v }},
	{"output", func(c Config) string { return c.Output }, func(c *Config, v string) { c.Output = v }},
	{"search_provider", func(c Config) string { return c.SearchProvider }, func(c *Config, v string) { c.SearchProvider = v }},
	{"search_api_key", func(c Config) string { return c.SearchAPIKey }, func(c *Config, v string) { c.SearchAPIKey = v }},
	{"yolo", func(c Config) string { return fmt.Sprintf("%t", c.Yolo) }, func(c *Config, v string) { c.Yolo = v == "true" || v == "1" }},
	{"max_run_seconds", func(c Config) string { return fmt.Sprintf("%d", c.MaxRunSeconds) }, func(c *Config, v string) { setPositiveInt(&c.MaxRunSeconds, v) }},
	{"compact_trigger_tokens", func(c Config) string { return fmt.Sprintf("%d", c.CompactTriggerTokens) }, func(c *Config, v string) { setPositiveInt(&c.CompactTriggerTokens, v) }},
	{"compact_keep_tokens", func(c Config) string { return fmt.Sprintf("%d", c.CompactKeepTokens) }, func(c *Config, v string) { setPositiveInt(&c.CompactKeepTokens, v) }},
}

func cmdConfig(env []string, args []string, stdout, stderr *os.File) int {
	sub := "show"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "path":
		fmt.Fprintln(stdout, configPath(env))
		return exitOK
	case "show", "list":
		cfg, err := LoadConfig(env)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		for _, k := range configKeys {
			v := k.Get(cfg)
			if (k.Name == "token" || k.Name == "search_api_key") && v != "" {
				v = maskSecret(v)
			}
			fmt.Fprintf(stdout, "%s=%s\n", k.Name, v)
		}
		return exitOK
	case "get":
		if len(args) < 2 {
			fmt.Fprintln(stderr, "usage: gocloudagent config get <key>")
			return exitUsage
		}
		cfg, err := LoadConfig(env)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		for _, k := range configKeys {
			if k.Name == args[1] {
				fmt.Fprintln(stdout, k.Get(cfg))
				return exitOK
			}
		}
		fmt.Fprintf(stderr, "unknown key %q; known: %s\n", args[1], knownKeys())
		return exitUsage
	case "set":
		if len(args) < 3 {
			fmt.Fprintln(stderr, "usage: gocloudagent config set <key> <value>")
			return exitUsage
		}
		cfg, err := LoadConfig(env)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		found := false
		for _, k := range configKeys {
			if k.Name == args[1] {
				k.Set(&cfg, args[2])
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(stderr, "unknown key %q; known: %s\n", args[1], knownKeys())
			return exitUsage
		}
		path, err := SaveConfig(env, cfg)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		fmt.Fprintln(stdout, "wrote", path)
		return exitOK
	case "test":
		cfg := MergeEnv(mustConfig(env), env)
		if err := cfg.Validate(); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		client := &HTTPClient{Endpoint: cfg.Endpoint, Token: cfg.Token}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := client.Probe(ctx); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitAPI
		}
		fmt.Fprintln(stdout, "ok")
		return exitOK
	case "json":
		cfg, err := LoadConfig(env)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return exitConfig
		}
		b, _ := json.MarshalIndent(cfg.masked(), "", "  ")
		fmt.Fprintln(stdout, string(b))
		return exitOK
	default:
		fmt.Fprintf(stderr, "unknown config subcommand %q\n", sub)
		return exitUsage
	}
}
