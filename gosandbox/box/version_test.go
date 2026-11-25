// 文件说明：version_test.go — gosandbox/box 的单元测试（TestSessionRejectsProtocolVersion 等）
//
// 用法
//
//	go test ./gosandbox/box -run 'TestSessionRejectsProtocolVersion'
//
// 调用架构
//
//	[上游] gosandbox/cmd, gosandbox/vm/box
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/box/version_test.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] backpressure_test.go, copytree.go, deadline.go, egress_e2e_test.go, files.go 等 26 个
//
// 主要符号
//
//	TestSessionRejectsProtocolVersion(t *testing.T)

package box

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/linkxzhou/SimpleBase/gosandbox/proto"
)

func TestSessionRejectsProtocolVersion(t *testing.T) {
	for _, version := range []uint16{0, 2, proto.Version + 1} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			host, peer := net.Pipe()
			defer host.Close()
			defer peer.Close()
			sent := make(chan error, 1)
			go func() { sent <- proto.Send(peer, &proto.Msg{Kind: proto.KindHello, Version: version}) }()
			s, err := newSessionOn(context.Background(), Config{}, host)
			want := fmt.Sprintf("box: agent speaks protocol %d, want %d", version, proto.Version)
			if s != nil || err == nil || err.Error() != want {
				t.Fatalf("session=%v err=%v", s, err)
			}
			if err := <-sent; err != nil {
				t.Fatal(err)
			}
		})
	}
}
