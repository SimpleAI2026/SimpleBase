// 文件说明：scheduler.go — 依赖调度（planv6.8 §13.8，吸收 cloudagent NextTaskIndex/BlockedTasks）
//
// 任务图是「只能指向前序」的无环图（plan.Validate 保证）：
//   - Ready 返回 pending 且依赖全部 completed 的任务（人类任务按 waiting_human 就绪）；
//   - Blocked 报告因依赖失败/取消而无法推进的任务（依赖失败 → 上游置 blocked）；
//   - AllSettled 报告是否全部任务都到终态。
//
// 并发默认 1（§13.13：开启需观测配额与确认交互）；调度器本身只做选择，
// 执行循环负责状态迁移与事件。

package group

import "strings"

// readyTask 挑选下一个可执行任务：按 ref 顺序找第一个 pending 且依赖满足的任务。
// 人类任务（target_kind=human）由执行循环直接置 waiting_human，不经 Runner。
func readyTask(tasks []Task) (Task, bool) {
	status := taskStatusIndex(tasks)
	for _, t := range tasks {
		if t.Status != TaskPending || t.TargetKind == "human" {
			continue
		}
		if depsMet(t, status) {
			return t, true
		}
	}
	return Task{}, false
}

// humanReady 挑选一个待人工处理的任务（pending 的 human dispatch）。
func humanReady(tasks []Task) (Task, bool) {
	for _, t := range tasks {
		if t.Status == TaskPending && t.TargetKind == "human" && depsMet(t, taskStatusIndex(tasks)) {
			return t, true
		}
	}
	return Task{}, false
}

// depsMet 报告任务的依赖是否全部完成（blocked 依赖返回 false，由调用方处理）。
func depsMet(t Task, status map[string]string) bool {
	for _, dep := range t.DependsOn {
		if status[dep] != TaskCompleted {
			return false
		}
	}
	return true
}

func taskStatusIndex(tasks []Task) map[string]string {
	out := make(map[string]string, len(tasks))
	for _, t := range tasks {
		out[t.Ref] = t.Status
	}
	return out
}

// BlockedTasks 返回依赖已失败/取消而无法推进的任务 ref（结果置 blocked 前的判定）。
func BlockedTasks(tasks []Task) []string {
	status := taskStatusIndex(tasks)
	var out []string
	for _, t := range tasks {
		if t.Status != TaskPending {
			continue
		}
		for _, dep := range t.DependsOn {
			switch status[dep] {
			case TaskFailed, TaskCanceled, TaskInterrupted, TaskBlocked:
				out = append(out, t.Ref)
			}
		}
	}
	return out
}

// AllSettled 报告全部任务是否都在终态。
func AllSettled(tasks []Task) bool {
	for _, t := range tasks {
		if !TerminalTask(t.Status) {
			return false
		}
	}
	return true
}

// AnyRunning 报告是否有任务在执行。
func AnyRunning(tasks []Task) bool {
	for _, t := range tasks {
		if t.Status == TaskRunning {
			return true
		}
	}
	return false
}

// HasWaitingHuman 报告是否有人在待办。
func HasWaitingHuman(tasks []Task) bool {
	for _, t := range tasks {
		if t.Status == TaskWaitingHuman {
			return true
		}
	}
	return false
}

// Progress 归约当前进度（completed/total）。
func Progress(tasks []Task) (percent int, summary string) {
	if len(tasks) == 0 {
		return 0, ""
	}
	done := 0
	for _, t := range tasks {
		if t.Status == TaskCompleted {
			done++
		}
	}
	percent = done * 100 / len(tasks)
	var sb strings.Builder
	sb.WriteString(itoa(done))
	sb.WriteByte('/')
	sb.WriteString(itoa(len(tasks)))
	return percent, sb.String() + " tasks"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// contextPayload 拼接 context_from 引用的上游结果（[来自 @X 的结果] 段落）。
func contextPayload(t Task, results map[string]taskOutcome) string {
	if len(t.ContextFrom) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, ref := range t.ContextFrom {
		o, ok := results[ref]
		if !ok || o.content == "" {
			continue
		}
		sb.WriteString("[来自 @")
		sb.WriteString(o.agentID)
		sb.WriteString(" 的结果]\n")
		sb.WriteString(o.content)
		sb.WriteString("\n\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// taskOutcome 是一个已完成任务的结果（context 拼接用）。
type taskOutcome struct {
	agentID string
	content string
}
