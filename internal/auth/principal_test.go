// 文件说明：principal_test.go — internal/auth 的单元测试（TestPrincipalActor、TestActorConstants 等）
//
// 用法
//
//	go test ./internal/auth -run 'TestPrincipalActor|TestActorConstants'
//
// 调用架构
//
//	[上游] internal/agentbridge, internal/agentrun, internal/api, internal/api/perfbench 等 9 个
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/auth/principal_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] api_key_repository.go, authenticate_middleware_test.go, cache.go, cache_test.go, cached_user_test.go 等 31 个
//
// 主要符号
//
//	TestPrincipalActor(t *testing.T)
//	TestActorConstants(t *testing.T)

package auth

import "testing"

func TestPrincipalActor(t *testing.T) {
	cases := []struct {
		name string
		p    Principal
		want string
	}{
		{"user only", Principal{UserID: "u1"}, "user:u1"},
		{"api key only", Principal{APIKeyID: "k1"}, "key:k1"},
		{"user wins over key", Principal{UserID: "u1", APIKeyID: "k1"}, "user:u1"},
		{"empty", Principal{}, ""},
		{"delegated user", Principal{UserID: "u1", APIKeyID: "k1", Delegated: true}, "user:u1"},
		{"delegated key", Principal{APIKeyID: "k1", Delegated: true}, "key:k1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.Actor(); got != tc.want {
				t.Fatalf("Actor()=%q want %q", got, tc.want)
			}
		})
	}
}

func TestActorConstants(t *testing.T) {
	if EndUserActor("e1") != "enduser:e1" {
		t.Fatal(EndUserActor("e1"))
	}
	for _, a := range []string{ActorSystem, ActorScheduler, ActorTrigger, ActorCron} {
		if a == "" {
			t.Fatal("empty system actor")
		}
	}
	if ActorScheduler == ActorTrigger || ActorTrigger == ActorCron || ActorScheduler == ActorCron {
		t.Fatal("system actors must be distinct")
	}
}
