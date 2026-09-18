package importer

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"sync"
	"time"
)

// RegistryConfig 注册器配置
type RegistryConfig struct {
	EnableCaching              bool          // 是否启用缓存
	CacheTimeout               time.Duration // 缓存超时时间
	MaxCacheSize               int           // 最大缓存大小
	EnableValidation           bool          // 是否启用验证
	AllowDuplicateRegistration bool          // 是否允许重复注册
}

// DefaultConfig 默认配置
var DefaultConfig = &RegistryConfig{
	EnableCaching:              true,
	CacheTimeout:               5 * time.Minute,
	MaxCacheSize:               1000,
	EnableValidation:           true,
	AllowDuplicateRegistration: false,
}

// Registry 包注册器
type Registry struct {
	mu             sync.RWMutex
	packages       map[string]*Package
	packagesByName map[string]*ast.ImportSpec
	externalTypes  sync.Map
	config         *RegistryConfig
}

// NewRegistry 创建新的包注册器
func NewRegistry() *Registry {
	return &Registry{
		packages:       make(map[string]*Package),
		packagesByName: make(map[string]*ast.ImportSpec),
		config:         DefaultConfig,
	}
}

// NewRegistryWithConfig 创建带配置的包注册器
func NewRegistryWithConfig(config *RegistryConfig) *Registry {
	if config == nil {
		config = DefaultConfig
	}
	return &Registry{
		packages:       make(map[string]*Package),
		packagesByName: make(map[string]*ast.ImportSpec),
		config:         config,
	}
}

// RegisterPackage 注册包
func (r *Registry) RegisterPackage(path, name string, objects ...*Object) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.config.AllowDuplicateRegistration {
		if _, exists := r.packages[path]; exists {
			return fmt.Errorf("package %s already registered", path)
		}
	}

	pkg := &Package{
		Path:    path,
		Name:    name,
		Objects: objects,
	}

	r.packages[path] = pkg
	r.packagesByName[name] = &ast.ImportSpec{
		Path: &ast.BasicLit{
			Kind:  token.STRING,
			Value: fmt.Sprintf(`"%s"`, path),
		},
	}

	return nil
}

// CreateFunction 创建函数对象
func (r *Registry) CreateFunction(name string, fn interface{}, documentation string) *Object {
	return &Object{
		Name:          name,
		Kind:          FunctionKind,
		Value:         reflect.ValueOf(fn),
		Type:          reflect.TypeOf(fn),
		Documentation: documentation,
	}
}

// CreateVariable 创建变量对象
func (r *Registry) CreateVariable(name string, valueAddr interface{}, valueType reflect.Type, documentation string) *Object {
	return &Object{
		Name:          name,
		Kind:          VariableKind,
		Value:         reflect.ValueOf(valueAddr),
		Type:          valueType,
		Documentation: documentation,
	}
}

// CreateConstant 创建常量对象
func (r *Registry) CreateConstant(name string, value interface{}, documentation string) *Object {
	return &Object{
		Name:          name,
		Kind:          ConstantKind,
		Value:         reflect.ValueOf(value),
		Type:          reflect.TypeOf(value),
		Documentation: documentation,
	}
}

// CreateType 创建类型对象
func (r *Registry) CreateType(name string, valueType reflect.Type, documentation string) *Object {
	return &Object{
		Name:          name,
		Kind:          TypeKind,
		Type:          valueType,
		Documentation: documentation,
	}
}

// GetPackage 获取包信息
func (r *Registry) GetPackage(path string) (*Package, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	pkg, exists := r.packages[path]
	return pkg, exists
}

// GetPackageByName 根据名称获取包信息
func (r *Registry) GetPackageByName(name string) (*ast.ImportSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	spec, exists := r.packagesByName[name]
	return spec, exists
}

// GetAllPackages 获取所有包
func (r *Registry) GetAllPackages() map[string]*Package {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*Package, len(r.packages))
	for k, v := range r.packages {
		result[k] = v
	}
	return result
}

// GetConfig 获取配置
func (r *Registry) GetConfig() *RegistryConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.config
}

// UpdateConfig 更新配置
func (r *Registry) UpdateConfig(config *RegistryConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config = config
}

// SetExternalType 设置外部类型映射
func (r *Registry) SetExternalType(goType types.Type, reflectType reflect.Type) {
	r.externalTypes.Store(goType.String(), reflectType)
}

// GetExternalType 获取外部类型映射
func (r *Registry) GetExternalType(goType types.Type) (reflect.Type, bool) {
	value, exists := r.externalTypes.Load(goType.String())
	if !exists {
		return nil, false
	}
	return value.(reflect.Type), true
}

// 全局注册器实例
var GlobalRegistry = NewRegistry()

// 全局便捷函数

// RegisterPackage 注册包到全局注册器
func RegisterPackage(path, name string, objects ...*Object) error {
	return GlobalRegistry.RegisterPackage(path, name, objects...)
}

// CreateFunction 创建函数对象（全局）
func CreateFunction(name string, fn interface{}, documentation string) *Object {
	return GlobalRegistry.CreateFunction(name, fn, documentation)
}

// CreateVariable 创建变量对象（全局）
func CreateVariable(name string, valueAddr interface{}, valueType reflect.Type, documentation string) *Object {
	return GlobalRegistry.CreateVariable(name, valueAddr, valueType, documentation)
}

// CreateConstant 创建常量对象（全局）
func CreateConstant(name string, value interface{}, documentation string) *Object {
	return GlobalRegistry.CreateConstant(name, value, documentation)
}

// CreateType 创建类型对象（全局）
func CreateType(name string, valueType reflect.Type, documentation string) *Object {
	return GlobalRegistry.CreateType(name, valueType, documentation)
}

// GetPackageByName 从全局注册器获取包信息
func GetPackageByName(name string) *ast.ImportSpec {
	spec, _ := GlobalRegistry.GetPackageByName(name)
	return spec
}

// GetAllPackages 从全局注册器获取所有包
func GetAllPackages() map[string]*Package {
	return GlobalRegistry.GetAllPackages()
}

// GetExternalType 从全局注册器获取外部类型
func GetExternalType(t types.Type) reflect.Type {
	typ, _ := GlobalRegistry.GetExternalType(t)
	return typ
}

// SetExternalType 设置外部类型到全局注册器
func SetExternalType(t types.Type, rType reflect.Type) {
	GlobalRegistry.SetExternalType(t, rType)
}
