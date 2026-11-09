// 文件说明：glob.go — gosandbox/host 中 MaxGlobMatches、ErrGlobTooLarge 等符号的实现
//
// 用法
//
//	ExpandGlob(pattern string) ([]string, error)  // ExpandGlob 展开 glob 为当下存在的路径（Linux 的快照语义）
//	type globKind（包内使用）
//	type globToken（包内使用）
//	同包调用：splitDenyEntry() → bwrap_args.go, policy.go 等；isGlobEntry() → bwrap_args.go, policy.go 等；ExpandGlob() → bwrap_args.go
//
// 调用架构
//
//	[上游] gocloudagent/cli, gocloudagent/tools, gosandbox/vm/local
//	         │ 调用本包导出 API
//	         ▼
//	[本文件] gosandbox/host/glob.go
//	         │
//	         ▼
//	[下游] 仅标准库 / 第三方
//	[同包] bwrap_args.go, cgroup.go, mandatory.go, policy.go, probe.go 等 13 个
//	[关键引用] splitDenyEntry() ← bwrap_args.go, policy.go, seatbelt.go
//
// 主要符号
//
//	const MaxGlobMatches
//	var ErrGlobTooLarge
//	ExpandGlob(pattern string) ([]string, error)
//	... 另有 15 个顶层符号

package host

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// literalPrefix 标记 DenyRead 条目按字面路径处理，不解释通配符。
// 用于 deny 名字里恰好带 * ? [ 的真实路径。
const literalPrefix = "literal:"

const (
	// MaxGlobMatches 是单个 glob 在 Linux 上展开的路径数上限；超限报错，不静默截断。
	MaxGlobMatches = 1024
	// maxGlobWalk 是单个 glob 展开时最多访问的目录项数，防止 /** 这类模式遍历整盘。
	maxGlobWalk = 200_000
)

// ErrGlobTooLarge 表示 glob 展开超出 MaxGlobMatches 或 maxGlobWalk。
var ErrGlobTooLarge = errors.New("host: deny_read glob expands too widely")

type globKind int

const (
	gText globKind = iota + 1
	gSep
	gAny
	gStar
	gGlobstar
	gGlobstarSlash
	gSet
)

type globToken struct {
	kind globKind
	text string // gText 的文本，gSet 的成员
}

// splitDenyEntry 去掉 literal: 前缀，返回路径与是否强制字面量。
func splitDenyEntry(entry string) (path string, literal bool) {
	if strings.HasPrefix(entry, literalPrefix) {
		return strings.TrimPrefix(entry, literalPrefix), true
	}
	return entry, false
}

// isGlobEntry 报告 DenyRead 条目是否按 glob 解释。
func isGlobEntry(entry string) bool {
	path, literal := splitDenyEntry(entry)
	return !literal && strings.ContainsAny(path, "*?[")
}

// setValid 报告方括号里的成员能否拼成字符集：非空且范围不倒置。
// 对齐 srt setRanges：[user-id] 里 r-i 倒置，整体按字面量。
func setValid(members string) bool {
	m := []rune(members)
	if len(m) == 0 {
		return false
	}
	for i := 0; i < len(m); i++ {
		isRange := i+2 < len(m) && m[i+1] == '-'
		from := m[i]
		if isRange {
			i += 2
		}
		if m[i] < from {
			return false
		}
	}
	return true
}

// parseGlob 把模式切成 token，每个字符串都是合法模式，没有转义字符（对齐 srt parseGlob）：
//   - [ 到其后第一个 ] 之间是一个集合，^ 和 ! 不取反；
//   - 拼不出集合的方括号（空、范围倒置、没有 ]）按字面文本；
//   - 连续的 * 合并到最少，避免正则回溯爆炸。
func parseGlob(pattern string) []globToken {
	r := []rune(pattern)
	var toks []globToken
	textStart := 0
	endText := func(at int) {
		if textStart < at {
			toks = append(toks, globToken{kind: gText, text: string(r[textStart:at])})
		}
	}
	kindAt := func(fromEnd int) globKind {
		if idx := len(toks) - fromEnd; idx >= 0 {
			return toks[idx].kind
		}
		return 0
	}
	addWildcard := func(kind globKind) {
		if kindAt(1) == gGlobstar || kindAt(1) == kind {
			return
		}
		isGlobstar := kind == gGlobstar ||
			(kindAt(2) == kind && (kindAt(1) == gStar || kindAt(1) == gGlobstarSlash))
		if !isGlobstar {
			toks = append(toks, globToken{kind: kind})
			return
		}
		for kindAt(1) == gStar || kindAt(1) == gGlobstarSlash {
			toks = toks[:len(toks)-1]
		}
		toks = append(toks, globToken{kind: gGlobstar})
	}

	bracketsAreTextUntil := 0
	for i := 0; i < len(r); {
		switch r[i] {
		case '/':
			endText(i)
			toks = append(toks, globToken{kind: gSep})
			i++
			textStart = i
		case '?':
			endText(i)
			toks = append(toks, globToken{kind: gAny})
			i++
			textStart = i
		case '*':
			endText(i)
			start := i
			for i < len(r) && r[i] == '*' {
				i++
			}
			withSep := i-start >= 2 && i < len(r) && r[i] == '/'
			rest := i - start
			if withSep {
				rest -= 2
			}
			if rest >= 2 {
				addWildcard(gGlobstar)
			}
			if rest == 1 {
				addWildcard(gStar)
			}
			if withSep {
				addWildcard(gGlobstarSlash)
				i++
			}
			textStart = i
		case '[':
			if i < bracketsAreTextUntil {
				i++
				break
			}
			closeAt := -1
			for j := i + 1; j < len(r); j++ {
				if r[j] == ']' {
					closeAt = j
					break
				}
			}
			if closeAt == -1 {
				bracketsAreTextUntil = len(r)
				i++
				break
			}
			members := string(r[i+1 : closeAt])
			if !setValid(members) {
				bracketsAreTextUntil = closeAt + 1
				i++
				break
			}
			endText(i)
			toks = append(toks, globToken{kind: gSet, text: members})
			i = closeAt + 1
			textStart = i
		default:
			i++
		}
	}
	endText(len(r))
	return toks
}

var setMemberEscaper = strings.NewReplacer(
	`.`, `\.`, `^`, `\^`, `$`, `\$`, `+`, `\+`, `{`, `\{`, `}`, `\}`,
	`(`, `\(`, `)`, `\)`, `|`, `\|`, `\`, `\\`,
)

func tokensRegex(toks []globToken) string {
	var b strings.Builder
	for _, t := range toks {
		switch t.kind {
		case gText:
			b.WriteString(regexp.QuoteMeta(t.text))
		case gSep:
			b.WriteByte('/')
		case gAny:
			b.WriteString(`[^/]`)
		case gStar:
			b.WriteString(`[^/]*`)
		case gGlobstar:
			b.WriteString(`.*`)
		case gGlobstarSlash:
			b.WriteString(`(.*/)?`)
		case gSet:
			b.WriteString("[" + setMemberEscaper.Replace(t.text) + "]")
		}
	}
	return b.String()
}

// splitGlob 把绝对 glob 拆成“无通配符的目录前缀”和“含通配符的余下部分”。
// 前缀不含末尾 /，根目录时为空串。
func splitGlob(pattern string) (prefix, rest string) {
	idx := strings.IndexAny(pattern, "*?[")
	if idx < 0 {
		return filepath.Clean(pattern), ""
	}
	slash := strings.LastIndexByte(pattern[:idx], '/')
	return strings.TrimRight(pattern[:slash], "/"), pattern[slash+1:]
}

// denyGlobRegex 返回 glob 的 deny 正则：^<前缀>/<余下>(/.*)?$。
// 前缀按 canonicalPrefix 处理（macOS 传 canonical，其余传 nil），
// 末尾 (/.*)? 让 deny 覆盖每个命中项之下的整棵子树。
func denyGlobRegex(pattern string, canonicalPrefix func(string) string) (string, error) {
	prefix, rest := splitGlob(pattern)
	if canonicalPrefix != nil && prefix != "" {
		prefix = canonicalPrefix(prefix)
	}
	expr := "^" + regexp.QuoteMeta(prefix) + "/" + tokensRegex(parseGlob(rest)) + "(/.*)?$"
	if _, err := regexp.Compile(expr); err != nil {
		return "", fmt.Errorf("host: deny_read glob %q is not usable: %w", pattern, err)
	}
	return expr, nil
}

// validateDenyEntry 校验单个 DenyRead 条目：必须绝对路径；glob 不得以分隔符结尾且能编译。
func validateDenyEntry(entry string) error {
	path, literal := splitDenyEntry(entry)
	if !filepath.IsAbs(path) {
		return fmt.Errorf("host: deny_read path %q must be absolute", entry)
	}
	if literal || !isGlobEntry(entry) {
		return nil
	}
	if strings.HasSuffix(path, "/") {
		return fmt.Errorf("host: deny_read glob %q must not end with a separator", entry)
	}
	_, err := denyGlobRegex(path, nil)
	return err
}

// globMatchesPath 报告 absPath 是否落在 glob 之内（含子树）。
func globMatchesPath(pattern, absPath string) bool {
	expr, err := denyGlobRegex(pattern, nil)
	if err != nil {
		return false
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return false
	}
	return re.MatchString(absPath)
}

// ExpandGlob 展开 glob 为当下存在的路径（Linux 的快照语义）。命中目录后不再下探，
// 因为目录整体会被盖掉。结果按路径排序，保证 bwrap 参数稳定。
func ExpandGlob(pattern string) ([]string, error) {
	prefix, rest := splitGlob(pattern)
	if rest == "" {
		return nil, nil
	}
	root := prefix
	if root == "" {
		root = "/"
	}
	if _, err := os.Lstat(root); err != nil {
		return nil, nil
	}
	full := "^" + regexp.QuoteMeta(prefix) + "/" + tokensRegex(parseGlob(rest)) + "$"
	re, err := regexp.Compile(full)
	if err != nil {
		return nil, fmt.Errorf("host: deny_read glob %q is not usable: %w", pattern, err)
	}
	// 没有 ** 和集合时，层数由模式决定，超出的层不必遍历。
	maxDepth := -1
	if !strings.Contains(rest, "**") && !strings.Contains(rest, "[") {
		maxDepth = strings.Count(rest, "/") + 1
	}

	var out []string
	visited := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if path == root {
			return nil
		}
		visited++
		if visited > maxGlobWalk {
			return fmt.Errorf("%w: %q visited more than %d entries", ErrGlobTooLarge, pattern, maxGlobWalk)
		}
		if re.MatchString(path) {
			out = append(out, path)
			if len(out) > MaxGlobMatches {
				return fmt.Errorf("%w: %q matched more than %d paths", ErrGlobTooLarge, pattern, MaxGlobMatches)
			}
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() && maxDepth >= 0 {
			rel := strings.TrimPrefix(strings.TrimPrefix(path, root), "/")
			if strings.Count(rel, "/")+1 >= maxDepth {
				return fs.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}
