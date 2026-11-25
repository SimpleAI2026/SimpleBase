// 文件说明：access_object.go — 宿主对象存储访问适配（实现 ObjectAccess 端口）
//
// 用法
//
//	NewCloudAgentObj(files objectstore.FileStore, index ObjectIndex) ObjectAccess  // 构造；两者皆空返回 nil
//	(a *cloudAgentObj) ListObjects(ctx, projectID, prefix, limit)  // 先查系统库索引，索引为空再回退列目录
//	(a *cloudAgentObj) HeadObject(ctx, projectID, key)  // 单个对象元信息
//
// 调用架构
//
//	[上游] internal/app（装配进 RuntimeProvider.Obj）
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] internal/agentbridge/access_object.go
//	         │ 依赖
//	         ▼
//	[下游] internal/objectstore, internal/systemdb
//	[同包] ports.go
//
// 主要符号
//
//	type ObjectIndex interface {…}
//	type cloudAgentObj struct {…}
//	NewCloudAgentObj(files objectstore.FileStore, index ObjectIndex) ObjectAccess
//	... 另有 6 个顶层符号
//
// 归属（internal-ddd-hexagonal-implementation-plan §4）：实现 agentbridge.ObjectAccess 端口，
// 属于宿主 ↔ gocloudagent 的防腐层（自 internal/api 迁入）。
// 物理键布局 {projectID}/files/{userKey} 与索引优先、文件回退的读语义原样保留。

package agentbridge

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/linkxzhou/SimpleBase/internal/objectstore"
	"github.com/linkxzhou/SimpleBase/internal/systemdb"
)

// ObjectIndex 是系统库 S3 索引的只读查询；*systemdb.Store 满足。
type ObjectIndex interface {
	ListS3Objects(ctx context.Context, projectID, prefix string, limit int) ([]systemdb.S3Object, error)
	GetS3Object(ctx context.Context, projectID, key string) (systemdb.S3Object, error)
}

// cloudAgentObj 以「索引优先、文件存储回退」实现 ObjectAccess。
type cloudAgentObj struct {
	files objectstore.FileStore
	index ObjectIndex
}

// NewCloudAgentObj 构造适配器；两者皆空时返回 nil。
// 接口参数里的 nil 指针（如未启用的 *systemdb.Store）按空处理：typed-nil 进入接口后 `== nil` 判不出来。
func NewCloudAgentObj(files objectstore.FileStore, index ObjectIndex) ObjectAccess {
	if isNilPointer(files) {
		files = nil
	}
	if isNilPointer(index) {
		index = nil
	}
	if files == nil && index == nil {
		return nil
	}
	return &cloudAgentObj{files: files, index: index}
}

// isNilPointer 判断接口值是否为 nil 或持有 nil 指针。
func isNilPointer(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Ptr && rv.IsNil()
}

func (a *cloudAgentObj) ListObjects(ctx context.Context, projectID, prefix string, limit int) ([]ObjectInfo, error) {
	if a.index != nil {
		rows, err := a.index.ListS3Objects(ctx, projectID, prefix, limit)
		if err != nil {
			return nil, err
		}
		out := make([]ObjectInfo, 0, len(rows))
		for _, o := range rows {
			out = append(out, toAgentObjectInfo(o))
		}
		if len(out) > 0 || a.files == nil {
			return out, nil
		}
	}
	if a.files == nil {
		return nil, fmt.Errorf("object store is not configured")
	}
	listed, err := a.files.List(ctx, joinKey(projectID, prefix), limit)
	if err != nil {
		return nil, err
	}
	out := make([]ObjectInfo, 0, len(listed))
	for _, o := range listed {
		lm := ""
		if !o.LastModified.IsZero() {
			lm = o.LastModified.UTC().Format(time.RFC3339)
		}
		out = append(out, ObjectInfo{
			Key:          stripProject(projectID, o.Key),
			Size:         o.Size,
			LastModified: lm,
		})
	}
	return out, nil
}

func (a *cloudAgentObj) HeadObject(ctx context.Context, projectID, key string) (ObjectInfo, error) {
	if err := objectstore.ValidateFileKey(key); err != nil {
		return ObjectInfo{}, err
	}
	if a.index != nil {
		o, err := a.index.GetS3Object(ctx, projectID, key)
		if err == nil {
			return toAgentObjectInfo(o), nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return ObjectInfo{}, err
		}
	}
	if a.files == nil {
		return ObjectInfo{}, fmt.Errorf("object %s not found", key)
	}
	listed, err := a.files.List(ctx, joinKey(projectID, key), 16)
	if err != nil {
		return ObjectInfo{}, err
	}
	for _, o := range listed {
		rel := stripProject(projectID, o.Key)
		if rel == key || o.Key == joinKey(projectID, key) {
			lm := ""
			if !o.LastModified.IsZero() {
				lm = o.LastModified.UTC().Format(time.RFC3339)
			}
			return ObjectInfo{Key: key, Size: o.Size, LastModified: lm}, nil
		}
	}
	return ObjectInfo{}, fmt.Errorf("object %s not found", key)
}

func toAgentObjectInfo(o systemdb.S3Object) ObjectInfo {
	info := ObjectInfo{Key: o.Key, Size: o.Size, ETag: o.ETag, ContentType: o.ContentType}
	if !o.LastModified.IsZero() {
		info.LastModified = o.LastModified.UTC().Format(time.RFC3339)
	}
	return info
}

// joinKey 把用户可见 key 拼成物理键 {projectID}/files/{key}；key 为空时是目录本身。
func joinKey(projectID, key string) string {
	if key == "" {
		return projectID + "/files"
	}
	return projectID + "/files/" + key
}

// stripProject 从物理 key 中去掉 {projectID}/files/ 前缀，还原用户可见的 key。
func stripProject(projectID, key string) string {
	prefix := projectID + "/files/"
	if strings.HasPrefix(key, prefix) {
		return key[len(prefix):]
	}
	if key == projectID+"/files" {
		return ""
	}
	return key
}
