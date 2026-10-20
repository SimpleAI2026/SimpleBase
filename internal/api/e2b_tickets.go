package api

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

// 终端票据：浏览器的 WebSocket 不能自定义 Authorization 头，所以先用普通鉴权换一张一次性票据，
// 再用票据升级 WebSocket。票据 32 字节随机，只在内存里，单次使用，绑定主体 + 项目 + 沙盒，30 秒过期。
const (
	ticketTTL      = 30 * time.Second
	ticketMaxLive  = 1024
	ticketBytesLen = 32
)

type terminalTicket struct {
	PrincipalID string
	ProjectID   string
	SandboxID   string
	Expires     time.Time
}

type ticketStore struct {
	mu sync.Mutex
	m  map[string]terminalTicket
}

func newTicketStore() *ticketStore { return &ticketStore{m: map[string]terminalTicket{}} }

// Issue 签发票据；存量过多时先清理过期项，仍满则拒绝（防止被刷爆内存）。
func (s *ticketStore) Issue(t terminalTicket, now time.Time) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.m) >= ticketMaxLive {
		for k, v := range s.m {
			if !now.Before(v.Expires) {
				delete(s.m, k)
			}
		}
		if len(s.m) >= ticketMaxLive {
			return "", false
		}
	}
	var b [ticketBytesLen]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", false
	}
	tok := base64.RawURLEncoding.EncodeToString(b[:])
	t.Expires = now.Add(ticketTTL)
	s.m[tok] = t
	return tok, true
}

// RedeemFor 取走票据并立即作废（无论是否过期、是否匹配）。校验票据绑定的沙盒与请求一致；
// 项目取自票据本身，签发时已经过项目解析与权限校验。
func (s *ticketStore) RedeemFor(tok, sandboxID string, now time.Time) (terminalTicket, bool) {
	s.mu.Lock()
	t, ok := s.m[tok]
	delete(s.m, tok)
	s.mu.Unlock()
	if !ok || !now.Before(t.Expires) || t.SandboxID != sandboxID {
		return terminalTicket{}, false
	}
	return t, true
}
