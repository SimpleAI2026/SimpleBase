package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/gocloudagent"
)

func TestDbg16(t *testing.T) {
	env := newSkillRouterEnv(t)
	env.llm.reset([][]gocloudagent.StreamDelta{
		toolDeltas("c1", []string{"function", "list"}),
		textDeltas("done"),
	})
	thread := env.createThreadFor(t, "gofunction")
	body, _ := json.Marshal(map[string]any{"content": "x", "stream": true})
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj-1/agent-threads/"+thread+"/runs", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-router-e2e-key")
	rec := httptest.NewRecorder()
	env.echo.ServeHTTP(rec, req)
	t.Logf("code=%d", rec.Code)
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if strings.Contains(line, "tool_result") || strings.Contains(line, "\"error\"") {
			t.Logf("%s", line)
		}
	}
}
