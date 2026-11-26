// 文件说明：access.go — 集合行级访问规则（owner_read / owner_write）的模型、校验与判定。
//
// 用法
//
//	rule, err := collection.ParseAccessRule(rawJSON) // 空串或 {} 返回 off
//	rule.RestrictsRead()                            // owner_read / owner_write
//	rule.CanWrite(actor, owner, isAdmin)
//
// 调用架构
//
//	[上游] internal/api/datahttp
//	         │ 读取项目设置中的规则并调用本文件
//	         ▼
//	[本文件] internal/collection/access.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] pick.go 的 FieldNamePattern
//
// 主要符号
//
//	const AccessOff / AccessOwnerRead / AccessOwnerWrite
//	type AccessRule struct{…}
//	ParseAccessRule / (AccessRule) Validate / Enforced / RestrictsRead / RestrictsWrite
//	(AccessRule) OwnerID / StampOwner / CanRead / CanWrite

package collection

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// AccessMode 是集合行级访问模式。
type AccessMode string

const (
	// AccessOff 关闭行级规则（默认）。
	AccessOff AccessMode = "off"
	// AccessOwnerRead 只限制读：非管理员只能读取 owner 为自己的文档。
	AccessOwnerRead AccessMode = "owner_read"
	// AccessOwnerWrite 同时限制读写：非管理员只能读写 owner 为自己的文档。
	AccessOwnerWrite AccessMode = "owner_write"
	// DefaultOwnerField 是默认的 owner 字段名。
	DefaultOwnerField = "owner_id"
)

var (
	// ErrAccessRuleInvalid 表示规则 JSON 或字段名不合法。
	ErrAccessRuleInvalid = errors.New("access_rule_invalid")
)

// AccessRule 是「项目 + 集合」维度的行级访问规则。
type AccessRule struct {
	Mode       AccessMode `json:"mode"`
	OwnerField string     `json:"owner_field,omitempty"`
}

// DefaultAccessRule 返回关闭状态。
func DefaultAccessRule() AccessRule {
	return AccessRule{Mode: AccessOff, OwnerField: DefaultOwnerField}
}

// ParseAccessRule 解析设置值；空串、空对象与 null 视为默认关闭。
func ParseAccessRule(raw string) (AccessRule, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return DefaultAccessRule(), nil
	}
	var rule AccessRule
	if err := json.Unmarshal([]byte(raw), &rule); err != nil {
		return DefaultAccessRule(), fmt.Errorf("%w: %v", ErrAccessRuleInvalid, err)
	}
	rule = rule.Normalize()
	if err := rule.Validate(); err != nil {
		return DefaultAccessRule(), err
	}
	return rule, nil
}

// Normalize 补齐缺省值。
func (r AccessRule) Normalize() AccessRule {
	if r.Mode == "" {
		r.Mode = AccessOff
	}
	if strings.TrimSpace(r.OwnerField) == "" {
		r.OwnerField = DefaultOwnerField
	}
	return r
}

// Validate 校验模式与 owner 字段名。
func (r AccessRule) Validate() error {
	switch r.Mode {
	case AccessOff, AccessOwnerRead, AccessOwnerWrite:
	default:
		return fmt.Errorf("%w: unknown mode %q", ErrAccessRuleInvalid, r.Mode)
	}
	field := r.OwnerField
	if field == "" {
		field = DefaultOwnerField
	}
	if !FieldNamePattern.MatchString(field) {
		return fmt.Errorf("%w: owner field %q must match %s", ErrAccessRuleInvalid, field, FieldNamePattern.String())
	}
	switch field {
	case "id", "created_at", "updated_at":
		return fmt.Errorf("%w: owner field %q is reserved", ErrAccessRuleInvalid, field)
	}
	return nil
}

// Enforced 表示是否启用了任意行级限制。
func (r AccessRule) Enforced() bool {
	return r.Mode == AccessOwnerRead || r.Mode == AccessOwnerWrite
}

// RestrictsRead 表示是否需要过滤读。
func (r AccessRule) RestrictsRead() bool { return r.Enforced() }

// RestrictsWrite 表示是否需要校验写。
func (r AccessRule) RestrictsWrite() bool { return r.Mode == AccessOwnerWrite }

// OwnerID 读取文档中的 owner 值；只接受字符串，缺失或类型不符返回空串。
func (r AccessRule) OwnerID(doc map[string]any) string {
	field := r.OwnerField
	if field == "" {
		field = DefaultOwnerField
	}
	v, ok := doc[field]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// StampOwner 在创建时写入 owner。管理员可显式指定 owner；非管理员总是被覆盖为 actor。
func (r AccessRule) StampOwner(doc map[string]any, actor string, admin bool) {
	if doc == nil || !r.Enforced() {
		return
	}
	field := r.OwnerField
	if field == "" {
		field = DefaultOwnerField
	}
	if admin {
		if s, _ := doc[field].(string); s != "" {
			return
		}
	}
	doc[field] = actor
}

// CanRead 判断主体能否读取该 owner 的文档。
func (r AccessRule) CanRead(actor, owner string, admin bool) bool {
	if !r.RestrictsRead() || admin {
		return true
	}
	return owner != "" && owner == actor
}

// CanWrite 判断主体能否修改该 owner 的文档。
func (r AccessRule) CanWrite(actor, owner string, admin bool) bool {
	if !r.RestrictsWrite() || admin {
		return true
	}
	return owner != "" && owner == actor
}

// PreserveOwner 在更新时把 owner 恢复为原值，避免非管理员把行转给他人。
func (r AccessRule) PreserveOwner(doc map[string]any, previous map[string]any, admin bool) {
	if doc == nil || !r.Enforced() || admin {
		return
	}
	field := r.OwnerField
	if field == "" {
		field = DefaultOwnerField
	}
	if v, ok := previous[field]; ok {
		doc[field] = v
		return
	}
	delete(doc, field)
}
