package cloudagent

import (
	"context"
	"strings"

	"github.com/linkxzhou/SimpleBase/gocloudagent/mcp"
)

// MCPConnector establishes the sessions selected for one run. Credentials and
// network policy belong to the connector implementation.
type MCPConnector interface {
	Connect(ctx context.Context, req RunRequest, serverIDs []string) (mcp.SessionSet, error)
}

// MCPPolicy classifies a selected MCP tool. Unknown tools must fail closed.
type MCPPolicy interface {
	Classify(ctx context.Context, req RunRequest, serverID, tool string) (ToolDecision, error)
}

// MCPToolRef selects a whole MCP server or one of its tools.
type MCPToolRef struct {
	ServerID string
	Tool     string
}

func mcpToolRefs(ids []string) []MCPToolRef {
	out := make([]MCPToolRef, 0, len(ids))
	for _, id := range ids {
		serverID, tool, ok := mcp.ParseToolID(id)
		if ok {
			out = append(out, MCPToolRef{ServerID: serverID, Tool: tool})
		}
	}
	return out
}

func mcpServerIDs(refs []MCPToolRef) []string {
	seen := make(map[string]bool, len(refs))
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if seen[ref.ServerID] {
			continue
		}
		seen[ref.ServerID] = true
		out = append(out, ref.ServerID)
	}
	return out
}

// HasMCPTools reports whether tool IDs contain an MCP reference.
func HasMCPTools(ids []string) bool {
	for _, id := range ids {
		if mcp.IsMCPToolID(id) {
			return true
		}
	}
	return false
}

func mcpToolFilter(refs []MCPToolRef, serverID string) (all bool, only map[string]bool) {
	only = make(map[string]bool)
	for _, ref := range refs {
		if ref.ServerID != serverID {
			continue
		}
		if strings.TrimSpace(ref.Tool) == "" {
			all = true
			continue
		}
		only[ref.Tool] = true
	}
	return all, only
}
