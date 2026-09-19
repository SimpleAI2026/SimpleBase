package packages

import (
	"strings"

	"github.com/linkxzhou/SimpleBase/gofunction/importer"
)

// stdStrings strings 包注册清单
func stdStrings() []*importer.Object {
	return []*importer.Object{
		funcObj("ToUpper", strings.ToUpper),
		funcObj("ToLower", strings.ToLower),
		funcObj("Contains", strings.Contains),
		funcObj("HasPrefix", strings.HasPrefix),
		funcObj("HasSuffix", strings.HasSuffix),
		funcObj("Split", strings.Split),
		funcObj("SplitN", strings.SplitN),
		funcObj("Fields", strings.Fields),
		funcObj("Join", strings.Join),
		funcObj("Replace", strings.Replace),
		funcObj("ReplaceAll", strings.ReplaceAll),
		funcObj("TrimSpace", strings.TrimSpace),
		funcObj("Trim", strings.Trim),
		funcObj("Index", strings.Index),
		funcObj("Count", strings.Count),
		funcObj("Repeat", strings.Repeat),
	}
}
