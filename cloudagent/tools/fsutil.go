package tools

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// skipDirs 是遍历时跳过的目录（与 §5.3 的「不进依赖与产物目录」一致）。
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true,
	"coverage": true, ".next": true, ".venv": true, "__pycache__": true,
}

// maxWalkFiles 限制单次遍历的文件数，避免在大仓库里卡死。
const maxWalkFiles = 20000

// globWalk 在 root 下按 pattern 匹配相对路径。
//
// 自己实现而不引第三方：模式支持 **、*、?，语义与仓库既有 doublestar 用法一致，
// 但不新增依赖（planv6.0 §12 Q3 的同一权衡）。
func globWalk(root, pattern string) ([]string, error) {
	return globWalkSkip(root, pattern, nil)
}

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

// grepWalk 在 root 下逐行匹配正则，返回 "file:line: text"。
func grepWalk(root string, re *regexp.Regexp, glob string, limit int) ([]string, bool, error) {
	return grepWalkSkip(root, re, glob, limit, nil)
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
			hits = append(hits, rel+":"+itoa(lineNo)+": "+strings.TrimSpace(line))
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

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
