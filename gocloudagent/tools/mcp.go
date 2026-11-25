// 文件说明：mcp.go — MCP 工具与 eino 工具之间的共享转换（planv6.8 §13.6）
//
// 云端（cloudagent/mcptool.go）与本地（cli/mcp_adapter.go）的授权、截断、脱敏策略不同，
// 各自保留；这里只收敛「MCP input schema → eino ToolInfo」这一份纯转换：
//   - MCPToolInfo(strict=true)：非法或非 object schema 返回错误（云端跳过该工具）；
//   - MCPToolInfo(strict=false)：非法 schema 退化为无参数工具（本地仍可调用）。

package tools

import (
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"

	"github.com/linkxzhou/SimpleBase/gocloudagent/mcp"
)

// MCPToolInfo 把 MCP 工具描述转成 eino ToolInfo；Extra["mcp_tool"] 保存原始工具名。
func MCPToolInfo(name string, info mcp.ToolInfo, strict bool) (*schema.ToolInfo, error) {
	ti := &schema.ToolInfo{
		Name:  name,
		Desc:  info.Description,
		Extra: map[string]any{"mcp_tool": info.Name},
	}
	var js jsonschema.Schema
	err := json.Unmarshal(info.InputSchema, &js)
	if err == nil && js.Type != "" && js.Type != "object" {
		err = fmt.Errorf("mcp: %s has non-object input schema (%q)", name, js.Type)
	}
	if err != nil {
		if strict {
			return nil, err
		}
		return ti, nil
	}
	ti.ParamsOneOf = schema.NewParamsOneOfByJSONSchema(&js)
	return ti, nil
}
