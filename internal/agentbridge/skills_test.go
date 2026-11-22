package agentbridge_test

import (
	"sort"
	"strings"
	"testing"

	agentskills "github.com/linkxzhou/SimpleBase/internal/agentbridge/agentskills"
	"github.com/linkxzhou/SimpleBase/internal/sbcli"
)

// 技能文档里的 `<!-- readonly: … -->` 标注必须与宿主 sbcli 判定一致，
// 否则模型会按文档把写命令当成只读。
func TestSkillReadOnlyAnnotationsMatchSbcli(t *testing.T) {
	annotations, err := agentskills.EmbeddedReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := annotations["index"]; ok {
		t.Fatal("索引不应有 readonly 标注")
	}
	if _, ok := annotations["kv"]; ok {
		t.Fatal("kv 不应对 readonly 做任何声明")
	}
	for id, cmds := range annotations {
		allows := agentskills.Prefixes(id)
		for _, cmd := range cmds {
			parts := strings.SplitN(cmd, " ", 2)
			if len(parts) != 2 {
				t.Fatalf("skill %s 的 readonly 项格式错误: %q", id, cmd)
			}
			allowed := false
			for _, p := range allows {
				if p == parts[0] {
					allowed = true
					break
				}
			}
			if !allowed {
				t.Fatalf("skill %s 标注了不属于自己的命令 %q", id, cmd)
			}
			if !sbcli.CommandReadOnlyWithArgv(parts) {
				t.Fatalf("skill %s 标注 %q 为只读，但 sbcli 判定为写", id, cmd)
			}
		}
	}

	for _, id := range agentskills.IDs() {
		body := agentskills.Body(id)
		declared := map[string]bool{}
		for _, c := range annotations[id] {
			declared[c] = true
		}
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "simplebase ") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 3 || !sbcli.CommandReadOnly(fields[1], fields[2]) {
				continue
			}
			if !declared[fields[1]+" "+fields[2]] {
				t.Fatalf("skill %s 的只读命令 %q 未在 readonly 标注中登记", id, fields[1]+" "+fields[2])
			}
		}
	}
}

func TestSkillCommandsDispatchable(t *testing.T) {
	for _, id := range agentskills.IDs() {
		body := agentskills.Body(id)
		if body == "" {
			t.Fatalf("skill %s 正文缺失", id)
		}
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "simplebase ") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 3 && !sbcli.DispatchKnown(fields[1], fields[2]) {
				t.Fatalf("skill %s 的示例无法分发: %q", id, line)
			}
		}
	}
}

func TestEmbeddedAllowsMatchPrefixes(t *testing.T) {
	allows, err := agentskills.EmbeddedAllows()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range agentskills.IDs() {
		got := append([]string(nil), allows[id]...)
		want := agentskills.Prefixes(id)
		sort.Strings(got)
		sort.Strings(want)
		if len(got) != len(want) {
			t.Fatalf("skill %s allow=%v want=%v", id, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("skill %s allow=%v want=%v", id, got, want)
			}
		}
	}
}
