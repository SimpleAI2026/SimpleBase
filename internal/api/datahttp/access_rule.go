// 文件说明：access_rule.go — 集合行级访问规则的读写接口与请求期判定。
//
// 用法
//
//	h := NewDataHandler(svc, true).WithAccessRules(get, put)
//	GET/PUT/DELETE /databases/:databaseID/data/collections/:collection/access
//
// 调用架构
//
//	[上游] internal/api（路由）与 internal/app（注入项目设置读写）
//	         │ 调用本文件
//	         ▼
//	[本文件] internal/api/datahttp/access_rule.go
//	         │ 依赖
//	         ▼
//	[下游] internal/collection（规则模型）、internal/auth（权限判定）、internal/api/httpkit
//
// 主要符号
//
//	type settingGetter / settingPutter
//	(DataHandler) WithAccessRules / GetAccessRule / PutAccessRule / DeleteAccessRule
//	(DataHandler) loadAccessRule / ruleScope
//	isRowAdmin / forbiddenRow

package datahttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/linkxzhou/SimpleBase/internal/api/httpkit"
	"github.com/linkxzhou/SimpleBase/internal/audit"
	"github.com/linkxzhou/SimpleBase/internal/auth"
	"github.com/linkxzhou/SimpleBase/internal/collection"
)

// auditRecorder 是规则变更的审计端口（只记规则摘要）。
type auditRecorder interface {
	Record(ctx context.Context, e audit.Event) error
}

const accessRuleSettingPrefix = "collection.access."

type settingGetter func(ctx context.Context, projectID, key string) (string, error)
type settingPutter func(ctx context.Context, projectID, key, valueJSON string) error

// WithAccessRules 注入项目设置的读写端口；未注入时规则视为 off。
func (h *DataHandler) WithAccessRules(get settingGetter, put settingPutter) *DataHandler {
	h.getSetting = get
	h.putSetting = put
	return h
}

// WithAudit 注入规则变更的审计端口；未注入时跳过审计（测试与只读实例）。
func (h *DataHandler) WithAudit(rec auditRecorder) *DataHandler {
	h.audit = rec
	return h
}

func accessRuleSettingKey(name string) string { return accessRuleSettingPrefix + name }

// loadAccessRule 读取并解析规则。读取或解析失败必须 fail-closed，不得静默按 off 放行。
func (h *DataHandler) loadAccessRule(ctx context.Context, projectID, name string) (collection.AccessRule, error) {
	if h.getSetting == nil {
		return collection.DefaultAccessRule(), nil
	}
	raw, err := h.getSetting(ctx, projectID, accessRuleSettingKey(name))
	if err != nil {
		return collection.DefaultAccessRule(), err
	}
	return collection.ParseAccessRule(raw)
}

// ruleScope 返回当前请求的规则、主体标识与管理员标记。
func (h *DataHandler) ruleScope(c echo.Context, name string) (collection.AccessRule, string, bool, error) {
	pc, err := httpkit.RequireProject(c)
	if err != nil {
		return collection.DefaultAccessRule(), "", false, err
	}
	rule, err := h.loadAccessRule(c.Request().Context(), pc.ID, name)
	if err != nil {
		return collection.DefaultAccessRule(), "", false, err
	}
	p := httpkit.PrincipalOf(c)
	return rule, p.Actor(), isRowAdmin(p), nil
}

// isRowAdmin 判断主体是否绕过行级规则。
func isRowAdmin(p auth.Principal) bool {
	return p.HasPermission(auth.DatabaseAdmin) || p.HasPermission(auth.ProjectAdmin)
}

// GetAccessRule 返回集合的行级规则。
func (h *DataHandler) GetAccessRule(c echo.Context) error {
	name := c.Param("collection")
	rule, err := h.loadAccessRule(c.Request().Context(), httpkit.ProjectID(c), name)
	if err != nil {
		return h.ruleError(c, err)
	}
	return c.JSON(http.StatusOK, accessRuleResponse(rule))
}

// PutAccessRule 覆盖集合的行级规则，仅项目管理员可调用（路由层已校验）。
func (h *DataHandler) PutAccessRule(c echo.Context) error {
	name := c.Param("collection")
	var req collection.AccessRule
	if err := c.Bind(&req); err != nil {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusBadRequest, "invalid_request", "invalid access rule", httpkit.RequestID(c)))
	}
	rule := req.Normalize()
	if err := rule.Validate(); err != nil {
		return h.ruleError(c, err)
	}
	if schema, err := h.schema(c.Request().Context(), name); err == nil && schema != nil && len(schema.Fields) > 0 {
		found := false
		for _, field := range schema.Fields {
			if field.Name == rule.OwnerField {
				found = true
				break
			}
		}
		if !found {
			return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusBadRequest, "invalid_request", "owner field must be a collection field", httpkit.RequestID(c)))
		}
	}
	if h.putSetting == nil {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusServiceUnavailable, "access_rule_unavailable", "access rules are not configured", httpkit.RequestID(c)))
	}
	payload, err := json.Marshal(rule)
	if err != nil {
		return h.ruleError(c, err)
	}
	if err := h.putSetting(c.Request().Context(), httpkit.ProjectID(c), accessRuleSettingKey(name), string(payload)); err != nil {
		return h.ruleError(c, err)
	}
	h.recordAccessChange(c, name, rule)
	return c.JSON(http.StatusOK, accessRuleResponse(rule))
}

// DeleteAccessRule 把规则恢复为 off。
func (h *DataHandler) DeleteAccessRule(c echo.Context) error {
	name := c.Param("collection")
	if h.putSetting == nil {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusServiceUnavailable, "access_rule_unavailable", "access rules are not configured", httpkit.RequestID(c)))
	}
	rule := collection.DefaultAccessRule()
	payload, _ := json.Marshal(rule)
	if err := h.putSetting(c.Request().Context(), httpkit.ProjectID(c), accessRuleSettingKey(name), string(payload)); err != nil {
		return h.ruleError(c, err)
	}
	h.recordAccessChange(c, name, rule)
	return c.JSON(http.StatusOK, accessRuleResponse(rule))
}

func (h *DataHandler) recordAccessChange(c echo.Context, name string, rule collection.AccessRule) {
	if h.audit == nil {
		return
	}
	_ = h.audit.Record(c.Request().Context(), audit.Event{
		ProjectID:   httpkit.ProjectID(c),
		PrincipalID: httpkit.PrincipalOf(c).Actor(),
		Kind:        "config_change",
		RequestID:   httpkit.RequestID(c),
		Status:      "ok",
		Detail:      "collection.access " + name + " mode=" + string(rule.Mode) + " owner=" + rule.OwnerField,
	})
}

type accessRuleResponseBody struct {
	Mode       collection.AccessMode `json:"mode"`
	OwnerField string                `json:"owner_field"`
	Enforced   bool                  `json:"enforced"`
}

func accessRuleResponse(rule collection.AccessRule) accessRuleResponseBody {
	if strings.TrimSpace(rule.OwnerField) == "" {
		rule.OwnerField = collection.DefaultOwnerField
	}
	return accessRuleResponseBody{Mode: rule.Mode, OwnerField: rule.OwnerField, Enforced: rule.Enforced()}
}

func (h *DataHandler) ruleError(c echo.Context, err error) error {
	if errors.Is(err, collection.ErrAccessRuleInvalid) {
		return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusInternalServerError, "access_rule_invalid", "collection access rule is invalid", httpkit.RequestID(c)))
	}
	if apiErr, ok := err.(*httpkit.APIError); ok {
		return httpkit.WriteError(c, apiErr)
	}
	return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusInternalServerError, "access_rule_unavailable", "collection access rule is unavailable", httpkit.RequestID(c)))
}

func forbiddenRow(c echo.Context) error {
	return httpkit.WriteError(c, httpkit.NewAPIError(http.StatusForbidden, "forbidden_row", "row is not accessible", httpkit.RequestID(c)))
}
