package systemdb

import (
	"context"
	"testing"
)

func TestProviderCredsUpsertMergeAndMask(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	pid := "proj-creds"

	// 初始无凭证
	rows, err := store.ListLLMProviderCreds(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("want empty, got %d", len(rows))
	}

	// 首次写入
	if err := store.UpsertLLMProviderCred(ctx, pid, "openai", "gpt-4o-mini", map[string]string{
		"api_key": "sk-initial-key-123456",
		"base_url": " https://api.openai.com/v1 ",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetLLMProviderCred(ctx, pid, "openai")
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultModel != "gpt-4o-mini" || !got.Enabled {
		t.Fatalf("unexpected cred: %+v", got)
	}

	// secret 留空：保留旧 key；非 secret 覆盖
	if err := store.UpsertLLMProviderCred(ctx, pid, "openai", "gpt-4o", map[string]string{
		"api_key": "",
		"base_url": "https://relay.example.com/v1",
	}); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetLLMProviderCred(ctx, pid, "openai")
	if err != nil {
		t.Fatal(err)
	}
	if !containsJSONKey(got.CredentialsJSON, "sk-initial-key-123456") {
		t.Fatalf("secret should be preserved, got %s", got.CredentialsJSON)
	}
	if !containsJSONKey(got.CredentialsJSON, "https://relay.example.com/v1") {
		t.Fatalf("base_url should be updated, got %s", got.CredentialsJSON)
	}
	if got.DefaultModel != "gpt-4o" {
		t.Fatalf("default model want gpt-4o, got %s", got.DefaultModel)
	}

	// 脱敏视图：secret 掩码、非 secret 原文
	masked := MaskedLLMProviderCreds([]LLMProviderCred{got})
	if len(masked) != 1 {
		t.Fatalf("want 1 masked row, got %d", len(masked))
	}
	m := masked[0]
	if m.Provider != "openai" || !m.HasAPIKey || !m.Enabled {
		t.Fatalf("unexpected masked meta: %+v", m)
	}
	if m.Credentials["api_key"] == "sk-initial-key-123456" {
		t.Fatalf("api_key must be masked, got %s", m.Credentials["api_key"])
	}
	if m.Credentials["api_key"] != MaskSecretValue("sk-initial-key-123456") {
		t.Fatalf("api_key mask mismatch: %s", m.Credentials["api_key"])
	}
	if m.Credentials["base_url"] != "https://relay.example.com/v1" {
		t.Fatalf("non-secret should be raw, got %s", m.Credentials["base_url"])
	}

	// 删除
	if err := store.DeleteLLMProviderCred(ctx, pid, "openai"); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetLLMProviderCred(ctx, pid, "openai")
	if err != nil {
		t.Fatal(err)
	}
	if got.CredentialsJSON != "" {
		t.Fatalf("cred should be deleted, got %s", got.CredentialsJSON)
	}
}

func TestMaskSecretValueShort(t *testing.T) {
	if MaskSecretValue("") != "" {
		t.Fatal("empty should stay empty")
	}
	if MaskSecretValue("short") != "••••••••" {
		t.Fatalf("short value should be fully masked, got %s", MaskSecretValue("short"))
	}
}

func containsJSONKey(payload, want string) bool {
	return len(payload) > 0 && indexOf(payload, want) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
