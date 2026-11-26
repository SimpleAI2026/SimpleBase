// 文件说明：group_prompt.go — 本地群运行时的终端交互提示：整份计划的确认 / 编辑、人工待办回复与行读取
//
// 用法：由 group_runtime.go 的 submit / follow 和 repl.go 进入群（/group）时调用；读不到交互输入时按「未决定」处理，不空转。
// 调用架构：groupRuntime.submit / follow ──► promptPlanConfirm / promptHumanReply ──► Engine.ConfirmPlan / Engine.ReplyHuman。
// 主要符号：promptPlanConfirm、promptEdits、promptHumanReply、printWaitingHuman、readLine。

package cli

import (
	"context"
	"strings"

	"github.com/linkxzhou/SimpleBase/gocloudagent/group"
	"github.com/linkxzhou/SimpleBase/gocloudagent/plan"
)

// promptPlanConfirm 在终端就整份计划询问 [a]pprove/[e]dit/[r]eject；edit 只支持改某个 ref 的指令或移除某个 ref。
func (rt *groupRuntime) promptPlanConfirm(turn group.Turn) (bool, []plan.Edit) {
	d := turn.Decision
	rt.println("\n⚠ 这份计划需要你确认后才会执行：")
	if d.Confirm.Reason != "" {
		rt.printf("  原因：%s\n", d.Confirm.Reason)
	}
	if d.Confirm.Question != "" {
		rt.printf("  问题：%s\n", d.Confirm.Question)
	}
	if len(d.Confirm.Options) > 0 {
		rt.printf("  可选：%s\n", strings.Join(d.Confirm.Options, " / "))
	}
	for {
		rt.printf("[a]pprove / [e]dit / [r]eject: ")
		raw, ok := rt.readLine()
		if !ok {
			// 没有交互输入（stdin 已结束）：按拒绝处理，绝不空转。
			rt.println("\n没有收到决定，计划已拒绝。")
			return false, nil
		}
		choice := strings.ToLower(strings.TrimSpace(raw))
		switch choice {
		case "a", "approve", "y", "yes":
			return true, nil
		case "r", "reject", "n", "no":
			return false, nil
		case "e", "edit":
			if !d.Confirm.CanEdit() {
				rt.println("这份计划不允许编辑（editable=false）：只能批准或拒绝。")
				continue
			}
			return true, rt.promptEdits(d)
		default:
			rt.println("请输入 a（批准）、e（编辑后批准）或 r（拒绝）。")
		}
	}
}

// promptEdits 逐条询问要改的指令与要移除的 ref。
func (rt *groupRuntime) promptEdits(d plan.Decision) []plan.Edit {
	var edits []plan.Edit
	for _, dp := range d.Dispatches {
		rt.printf("  %s → @%s：%s\n", dp.Ref, dp.TargetID, firstLine(dp.Message, 120))
	}
	rt.printf("修改哪个 ref 的指令（回车跳过）: ")
	if raw, ok := rt.readLine(); ok {
		if ref := strings.TrimSpace(raw); ref != "" {
			rt.printf("新指令: ")
			if rawMsg, ok := rt.readLine(); ok {
				if msg := strings.TrimSpace(rawMsg); msg != "" {
					edits = append(edits, plan.Edit{Ref: ref, Message: &msg})
				}
			}
		}
	}
	rt.printf("移除哪个 ref（回车跳过）: ")
	if raw, ok := rt.readLine(); ok {
		if ref := strings.TrimSpace(raw); ref != "" {
			edits = append(edits, plan.Edit{Ref: ref, Remove: true})
		}
	}
	return edits
}

// promptHumanReply 就人工待办询问回复；返回 false 表示用户选择稍后处理（/reply）。
func (rt *groupRuntime) promptHumanReply(ctx context.Context, turnID string) bool {
	rt.printWaitingHuman(turnID)
	rt.printf("回复并继续请直接输入；无法处理输入 /unable [说明]；稍后处理输入 /later: ")
	raw, ok := rt.readLine()
	if !ok {
		rt.println("\n没有收到回复，待办已保留。")
		return false
	}
	line := strings.TrimSpace(raw)
	switch {
	case line == "" || line == "/later":
		rt.println("已保留待办：稍后用 /reply <说明> 或 /unable [说明] 处理。")
		return false
	case strings.HasPrefix(line, "/unable"):
		text := strings.TrimSpace(strings.TrimPrefix(line, "/unable"))
		_, taskID := rt.waitingHumanTask()
		if _, err := rt.Engine.ReplyHuman(ctx, taskID, group.Reply{Text: text, Outcome: group.ReplyUnable}); err != nil {
			rt.printf("error: %v\n", err)
		}
		return true
	default:
		_, taskID := rt.waitingHumanTask()
		if _, err := rt.Engine.ReplyHuman(ctx, taskID, group.Reply{Text: line, Outcome: group.ReplyContinue}); err != nil {
			rt.printf("error: %v\n", err)
		}
		return true
	}
}

func (rt *groupRuntime) printWaitingHuman(turnID string) {
	tasks, _ := rt.Store.LoadTasks(context.Background(), turnID)
	for _, t := range tasks {
		if t.Status == group.TaskWaitingHuman {
			rt.printf("等待你处理：%s（用 /reply <说明> 回复并继续，/unable [说明] 标记无法处理）\n", t.Message)
		}
	}
}

// readLine 从交互输入读一行；非交互（In 为 nil）或输入已结束时 ok=false。
func (rt *groupRuntime) readLine() (string, bool) {
	if rt.Runner.lineReader == nil {
		return "", false
	}
	line, err := rt.Runner.lineReader.ReadString('\n')
	if err != nil && line == "" {
		return "", false
	}
	return strings.TrimRight(line, "\r\n"), true
}
