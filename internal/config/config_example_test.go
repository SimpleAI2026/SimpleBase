package config

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestConfigExampleCoversAllKeys 验证 config.example.yaml：
//  1. 能被 yaml 反序列化为 Config（结构与真实配置一致）；
//  2. 每个显式设置的叶子键行都带注释（#），即每个配置项都有说明。
//
// 阶段 F（planv6.0 storage-local-first）：示例文件是配置项文档的权威来源。
func TestConfigExampleCoversAllKeys(t *testing.T) {
	data, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		t.Skipf("config.example.yaml not found: %v", err)
	}
	text := string(data)

	// 1. 反序列化：结构必须与 Config 对齐。
	var yc yamlConfig
	if err := yaml.Unmarshal(data, &yc); err != nil {
		t.Fatalf("parse config.example.yaml: %v", err)
	}

	// 2. 每个叶子键行必须带行内或上方注释。
	//    叶子行 = 形如 "  key: value" 的非容器行（无子节点）。
	keyRe := regexp.MustCompile(`^(\s+)([a-z_][a-z0-9_]*):\s*(\S.*?)?$`)
	nestedRe := regexp.MustCompile(`^(\s+)([a-z_][a-z0-9_]*):\s*$`)
	commentRe := regexp.MustCompile(`#`)
	var missing []string
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if nestedRe.MatchString(line) {
			continue // 容器段（如 ducklake:、lease:），注释由其子项体现
		}
		if !keyRe.MatchString(line) {
			continue
		}
		if commentRe.MatchString(line) {
			continue
		}
		// 看上方是否有紧邻的注释块。
		if i > 0 && strings.Contains(lines[i-1], "#") {
			continue
		}
		missing = append(missing, trimmed)
	}
	if len(missing) > 0 {
		t.Fatalf("config.example.yaml 中以下配置项缺少注释说明:\n  %s",
			strings.Join(missing, "\n  "))
	}
}

// TestConfigExampleNoDevMode 确认示例文件不再含 dev_mode。
func TestConfigExampleNoDevMode(t *testing.T) {
	data, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		t.Skipf("config.example.yaml not found: %v", err)
	}
	if strings.Contains(strings.ToLower(string(data)), "dev_mode") {
		t.Fatal("config.example.yaml still mentions dev_mode")
	}
}
