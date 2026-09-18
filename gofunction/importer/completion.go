package importer

import (
	"fmt"
	"strings"
)

// ObjectKindDisplayName 对象类型显示名称映射
var ObjectKindDisplayName = map[ObjectKind]string{
	VariableKind:        "Variable",
	ConstantKind:        "Constant",
	TypeKind:            "Type",
	FunctionKind:        "Function",
	BuiltinFunctionKind: "Function",
}

// CompletionItem 代码补全项
type CompletionItem struct {
	Label           string `json:"label"`           // 显示标签
	Kind            string `json:"kind"`            // 项目类型
	InsertText      string `json:"insertText"`      // 插入文本
	InsertTextRules string `json:"insertTextRules"` // 插入规则
	Documentation   string `json:"documentation"`   // 文档说明
}

// CompletionProvider 代码补全提供器
type CompletionProvider struct {
	registry *Registry
}

// NewCompletionProvider 创建代码补全提供器
func NewCompletionProvider(registry *Registry) *CompletionProvider {
	if registry == nil {
		registry = GlobalRegistry
	}
	return &CompletionProvider{
		registry: registry,
	}
}

// GetCompletionItems 获取所有已注册包的代码补全项
func (p *CompletionProvider) GetCompletionItems() []*CompletionItem {
	completionItems := make([]*CompletionItem, 0)

	for _, pkg := range p.registry.GetAllPackages() {
		for _, object := range pkg.Objects {
			item := &CompletionItem{
				Label:         fmt.Sprintf("%s.%s", pkg.Name, object.Name),
				Kind:          ObjectKindDisplayName[object.Kind],
				Documentation: object.Documentation,
			}

			if object.Kind == FunctionKind || object.Kind == BuiltinFunctionKind {
				item.InsertText = p.generateFunctionSnippet(pkg.Name, object)
				item.InsertTextRules = "InsertAsSnippet"
			} else {
				item.InsertText = fmt.Sprintf("%s.%s", pkg.Name, object.Name)
			}

			completionItems = append(completionItems, item)
		}
	}

	return completionItems
}

// GetPackageCompletions 获取指定包的代码补全项
func (p *CompletionProvider) GetPackageCompletions(packageName string) []*CompletionItem {
	completionItems := make([]*CompletionItem, 0)

	for _, pkg := range p.registry.GetAllPackages() {
		if pkg.Name == packageName {
			for _, object := range pkg.Objects {
				item := &CompletionItem{
					Label:         object.Name,
					Kind:          ObjectKindDisplayName[object.Kind],
					InsertText:    object.Name,
					Documentation: object.Documentation,
				}

				if object.Kind == FunctionKind || object.Kind == BuiltinFunctionKind {
					item.InsertText = p.generateFunctionSnippet("", object)
					item.InsertTextRules = "InsertAsSnippet"
				}

				completionItems = append(completionItems, item)
			}
			break
		}
	}

	return completionItems
}

// GetFunctionSignature 获取函数签名
func (p *CompletionProvider) GetFunctionSignature(packageName, functionName string) string {
	for _, pkg := range p.registry.GetAllPackages() {
		if pkg.Name == packageName {
			for _, object := range pkg.Objects {
				if object.Name == functionName && (object.Kind == FunctionKind || object.Kind == BuiltinFunctionKind) {
					return object.Type.String()
				}
			}
		}
	}
	return ""
}

// generateFunctionSnippet 生成函数代码片段
func (p *CompletionProvider) generateFunctionSnippet(packagePrefix string, object *Object) string {
	parameterPlaceholders := make([]string, 0)

	for i := 0; i < object.Type.NumIn(); i++ {
		paramType := object.Type.In(i)
		placeholder := fmt.Sprintf("${%d:%s}", i+1, p.getTypeDisplayName(paramType))
		parameterPlaceholders = append(parameterPlaceholders, placeholder)
	}

	functionName := object.Name
	if packagePrefix != "" {
		functionName = fmt.Sprintf("%s.%s", packagePrefix, object.Name)
	}

	return fmt.Sprintf("%s(%s)", functionName, strings.Join(parameterPlaceholders, ", "))
}

// getTypeDisplayName 获取类型的显示名称
func (p *CompletionProvider) getTypeDisplayName(reflectType interface{ String() string }) string {
	typeName := reflectType.String()

	// 简化常见类型名称
	switch typeName {
	case "int", "int8", "int16", "int32", "int64":
		return "int"
	case "uint", "uint8", "uint16", "uint32", "uint64":
		return "uint"
	case "float32", "float64":
		return "float"
	case "string":
		return "string"
	case "bool":
		return "bool"
	default:
		return typeName
	}
}

// 全局默认补全提供器实例
var GlobalCompletionProvider = NewCompletionProvider(nil)

// 全局便捷函数

// GetCompletionItems 获取代码补全项（全局）
func GetCompletionItems() []*CompletionItem {
	return GlobalCompletionProvider.GetCompletionItems()
}

// GetPackageCompletions 获取包补全项（全局）
func GetPackageCompletions(packageName string) []*CompletionItem {
	return GlobalCompletionProvider.GetPackageCompletions(packageName)
}

// GetFunctionSignature 获取函数签名（全局）
func GetFunctionSignature(packageName, functionName string) string {
	return GlobalCompletionProvider.GetFunctionSignature(packageName, functionName)
}

// 向后兼容的类型别名
type KeywordInfo = CompletionItem

// Keywords 获取关键字信息（向后兼容）
// Deprecated: 使用 GetCompletionItems() 替代
func Keywords() []*CompletionItem {
	return GetCompletionItems()
}