package agentbridge_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/linkxzhou/SimpleBase/gocloudagent"
	"github.com/linkxzhou/SimpleBase/internal/sbcli"
)

// 技能文档里的 `<!-- readonly: … -->` 标注必须与宿主 sbcli 判定一致，
// 否则模型会按文档把写命令当成只读。
//
// 对账放在宿主侧：gocloudagent 是独立模块，不 import sbcli；但它的 embed
// 文档通过公开 API 暴露，宿主的 sbcli 提供事实判定。
func TestSkillReadOnlyAnnotationsMatchSbcli(t *testing.T) {
	annotations, err := gocloudagent.EmbeddedSkillReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := annotations["index"]; ok {
		t.Fatal("索引不应有 readonly 标注")
	}
	// kv 全是写命令（exec 语义），不应出现标注。
	if _, ok := annotations["kv"]; ok {
		t.Fatal("kv 不应对 readonly 做任何声明")
	}
	for id, cmds := range annotations {
		allows := gocloudagent.SkillPrefixes(id)
		for _, cmd := range cmds {
			parts := strings.SplitN(cmd, " ", 2)
			if len(parts) != 2 {
				t.Fatalf("skill %s 的 readonly 项格式错误: %q", id, cmd)
			}
			// 标注的命令必须落在该 skill 的 allow 前缀内。
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

	// 反向：文档里出现的每个实际只读命令都必须被标注，防止漏标。
	for _, id := range gocloudagent.KnownSkillIDs() {
		body := gocloudagent.SkillBody(id)
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
			if len(fields) < 3 {
				continue
			}
			if !sbcli.CommandReadOnly(fields[1], fields[2]) {
				continue
			}
			if !declared[fields[1]+" "+fields[2]] {
				t.Fatalf("skill %s 的只读命令 %q 未在 readonly 标注中登记", id, fields[1]+" "+fields[2])
			}
		}
	}
}

// 文档示例里的每条 `simplebase <resource> <action>` 都必须能被 sbcli 分发。
func TestSkillCommandsDispatchable(t *testing.T) {
	for _, id := range gocloudagent.KnownSkillIDs() {
		body := gocloudagent.SkillBody(id)
		if body == "" {
			t.Fatalf("skill %s 正文缺失", id)
		}
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "simplebase ") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			if !sbcli.DispatchKnown(fields[1], fields[2]) {
				t.Fatalf("skill %s 的示例无法分发: %q", id, line)
			}
		}
	}
}

// 嵌入的 allow 注释必须与模块内的技能前缀表一致。
func TestEmbeddedAllowsMatchPrefixes(t *testing.T) {
	allows, err := gocloudagent.EmbeddedSkillAllows()
	if err != nil {
		t.Fatal(err)
	}
	ids := gocloudagent.KnownSkillIDs()
	for _, id := range ids {
		got := append([]string(nil), allows[id]...)
		want := gocloudagent.SkillPrefixes(id)
		// embed 的 allow 注释与内部前缀表顺序不同，比较前统一排序。
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
