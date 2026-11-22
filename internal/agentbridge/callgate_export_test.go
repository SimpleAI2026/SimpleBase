package agentbridge

// size 返回当前登记数（仅测试）。
func (g *callGate) size() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.slots)
}
