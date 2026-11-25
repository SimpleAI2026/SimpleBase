// 文件说明：skills.go — gocloudagent/skills 中 Catalog、Load 等符号的实现
//
// 用法
//
//	type Catalog struct{…}  // Catalog is an immutable collection of Markdown skill documents keyed by their ex...
//	Load(fsys fs.FS, root string) (*Catalog, error)  // Load reads all Markdown documents below root. YAML frontmatter is omitted.
//	ReadDocument(fsys fs.FS, filePath string) (string, error)  // ReadDocument 读取单个 Markdown 文档：去掉 YAML frontmatter 并裁掉首尾空白
//
// 调用架构
//
//	[上游] internal/agentbridge/agentskills
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/skills/skills.go
//	         │ 依赖
//	         ▼
//	[下游] 仅标准库
//	[同包] skills_test.go
//	[关键引用] Catalog ← skills_test.go；Load ← skills_test.go；ReadDocument ← skills_test.go
//
// 主要符号
//
//	type Catalog struct{…}
//	Load(fsys fs.FS, root string) (*Catalog, error)
//	(c *Catalog) IDs() []string
//	(c *Catalog) Body(id string) string
//	(c *Catalog) Prompt(ids ...string) string
//	ReadDocument(fsys fs.FS, filePath string) (string, error)
//	stripFrontmatter(body string) string

// Package skills loads prompt documents from any filesystem without prescribing
// a skill naming scheme, command policy, or host platform.
package skills

import (
	"io/fs"
	"path"
	"sort"
	"strings"
)

// Catalog is an immutable collection of Markdown skill documents keyed by their
// extensionless paths relative to the loaded root.
type Catalog struct {
	documents map[string]string
	ids       []string
}

// Load reads all Markdown documents below root. YAML frontmatter is omitted.
func Load(fsys fs.FS, root string) (*Catalog, error) {
	catalog := &Catalog{documents: map[string]string{}}
	root = strings.Trim(path.Clean(root), "/")
	if root == "" || root == "." {
		root = "."
	}
	err := fs.WalkDir(fsys, root, func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(filePath, ".md") {
			return nil
		}
		body, err := ReadDocument(fsys, filePath)
		if err != nil {
			return err
		}
		id := strings.TrimSuffix(strings.TrimPrefix(filePath, root+"/"), ".md")
		if root == "." {
			id = strings.TrimSuffix(strings.TrimPrefix(filePath, "./"), ".md")
		}
		catalog.documents[id] = body
		catalog.ids = append(catalog.ids, id)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(catalog.ids)
	return catalog, nil
}

// IDs returns all document IDs in lexical order.
func (c *Catalog) IDs() []string {
	if c == nil {
		return nil
	}
	return append([]string(nil), c.ids...)
}

// Body returns one document body.
func (c *Catalog) Body(id string) string {
	if c == nil {
		return ""
	}
	return c.documents[id]
}

// Prompt joins selected documents in the requested order. Unknown IDs are skipped.
func (c *Catalog) Prompt(ids ...string) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if body := c.Body(id); body != "" {
			parts = append(parts, body)
		}
	}
	return strings.Join(parts, "\n\n")
}

// ReadDocument 读取单个 Markdown 文档：去掉 YAML frontmatter 并裁掉首尾空白。
//
// 只处理以 LF 换行、以独立一行 "---" 结束的 frontmatter；其他形态原样保留。
func ReadDocument(fsys fs.FS, filePath string) (string, error) {
	body, err := fs.ReadFile(fsys, filePath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stripFrontmatter(string(body))), nil
}

func stripFrontmatter(body string) string {
	if !strings.HasPrefix(body, "---\n") {
		return body
	}
	end := strings.Index(body[4:], "\n---")
	if end < 0 {
		return body
	}
	return strings.TrimPrefix(body[4+end+len("\n---"):], "\n")
}
