package iamhttp

// api_key_policy_test.go：API Key 签发的权限子集判定（自 internal/api/policy_gate_test.go 随实现迁入）。

import (
	"testing"

	"github.com/linkxzhou/SimpleBase/internal/auth"
)

func TestScopeSubset(t *testing.T) {
	parent := &auth.KeyPolicy{Resources: []string{"collection:orders"}, Actions: []string{"read"}, HiddenFields: []string{"secret"}}
	if !scopeSubset(parent, parent) {
		t.Fatal("same scope rejected")
	}
	if scopeSubset(parent, &auth.KeyPolicy{Resources: []string{"collection:*"}, Actions: []string{"read"}}) {
		t.Fatal("wider resource accepted")
	}
	if scopeSubset(parent, &auth.KeyPolicy{Resources: []string{"collection:orders"}, Actions: []string{"read", "write"}}) {
		t.Fatal("wider action accepted")
	}
}

// 受限 Key（Scope 非空）经 g.require / g.delegable 挂载的路由必须先过 policyGate。
// function-triggers 过去直接用 auth.Require，绕开了 Key 的资源 / 动作范围。
