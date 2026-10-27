// 文件说明：fsutil.go — gocloudagent/tools 的内部实现（skipDirs、maxWalkFiles 等）
//
// 用法
//
//	type pathSkipper（包内使用）
//	同包调用：pathSkipper → tools.go；globWalkSkip() → tools.go；grepWalkSkip() → tools.go
//
// 调用架构
//
//	[上游] gocloudagent/cli
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gocloudagent/tools/fsutil.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] search.go, tools.go
//	[关键引用] pathSkipper ← tools.go
//
// 主要符号
//
//	var skipDirs
//	const maxWalkFiles
//	type pathSkipper func(absPath string) bool
//	globWalkSkip(root, pattern string, skip pathSkipper) ([]string, error)
//	matchGlob(pattern, name string) bool
//	globToRegex(pattern string) string
//	... 另有 3 个顶层符号

package tools

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// skipDirs 是遍历时跳过的目录（与 §5.3 的「不进依赖与产物目录」一致）。
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true,
	"coverage": true, ".next": true, ".venv": true, "__pycache__": true,
}

// maxWalkFiles 限制单次遍历的文件数，避免在大仓库里卡死。
const maxWalkFiles = 20000

// pathSkipper 判断一个绝对路径是否应被遍历跳过（隔离策略的 deny_read）。
type pathSkipper func(absPath string) bool

func globWalkSkip(root, pattern string, skip pathSkipper) ([]string, error) {
	pattern = filepath.ToSlash(pattern)
	var out []string
	count := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 忽略无权限的子树
		}
		if skip != nil && skip(path) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] && path != root {
				return fs.SkipDir
			}
			return nil
		}
		count++
		if count > maxWalkFiles {
			return fs.SkipAll
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if matchGlob(pattern, rel) {
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// matchGlob 把 glob 模式转成正则匹配（支持 ** 跨目录、* 单层、? 单字符）。
func matchGlob(pattern, name string) bool {
	re, err := regexp.Compile("^" + globToRegex(pattern) + "$")
	if err != nil {
		return false
	}
	return re.MatchString(name)
}

func globToRegex(pattern string) string {
	var b strings.Builder
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				// **/ 匹配零层或多层目录。
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					b.WriteString(`(?:.*/)?`)
				} else {
					b.WriteString(`.*`)
				}
			} else {
				b.WriteString(`[^/]*`)
			}
		case '?':
			b.WriteString(`[^/]`)
		case '.', '+', '(', ')', '|', '^', '$', '[', ']', '{', '}', '\\':
			b.WriteString(`\` + string(c))
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func grepWalkSkip(root string, re *regexp.Regexp, glob string, limit int, skip pathSkipper) ([]string, bool, error) {
	var files []string
	if strings.TrimSpace(glob) != "" {
		var err error
		files, err = globWalkSkip(root, glob, skip)
		if err != nil {
			return nil, false, err
		}
	} else {
		count := 0
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if skip != nil && skip(path) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				if skipDirs[d.Name()] && path != root {
					return fs.SkipDir
				}
				return nil
			}
			count++
			if count > maxWalkFiles {
				return fs.SkipAll
			}
			rel, rerr := filepath.Rel(root, path)
			if rerr != nil {
				return nil
			}
			files = append(files, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return nil, false, err
		}
	}
	var hits []string
	truncated := false
	for _, rel := range files {
		if len(hits) >= limit {
			return hits, true, nil
		}
		if isBinaryExt(rel) {
			continue
		}
		f, err := os.Open(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
		lineNo := 0
		for sc.Scan() {
			lineNo++
			line := sc.Text()
			if !re.MatchString(line) {
				continue
			}
			hits = append(hits, rel+":"+strconv.Itoa(lineNo)+": "+strings.TrimSpace(line))
			if len(hits) >= limit {
				// 命中上限即视为截断：后面可能还有更多匹配。
				truncated = true
				break
			}
		}
		f.Close()
		if truncated {
			break
		}
	}
	return hits, truncated, nil
}

// binaryExts 是 grep 直接跳过的扩展名。
var binaryExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".ico": true,
	".pdf": true, ".zip": true, ".gz": true, ".tar": true, ".woff": true, ".woff2": true,
	".ttf": true, ".so": true, ".dylib": true, ".dll": true, ".exe": true, ".bin": true,
	".mp3": true, ".mp4": true, ".mov": true, ".wasm": true, ".db": true,
}

func isBinaryExt(name string) bool {
	return binaryExts[strings.ToLower(filepath.Ext(name))]
}
