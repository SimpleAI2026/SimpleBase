// 文件说明：planner.go — Planner：调模型并强制以 plan 工具调用输出 Decision（planv6.8 §13.3/§13.5）
//
// 决策链：
//  1. 原生 function calling（Tools=[plan]）；模型不支持 tools（400/422 且文案含
//     tool/function）时回退文本协议，复用 agentcore 的 TOOL_CALL 单行格式；
//  2. 从响应提取 plan 调用（原生 ToolCalls 优先，其次 TOOL_CALL 文本）；
//  3. Normalize + Validate；校验失败做一次带错误信息的修复重试；
//  4. 仍失败返回 ErrNoPlan，由调用方（群引擎栅栏）走规则回退——被 @ 的目标各收原文
//     或 fallback 助手，保持可用。
//
// Planner 不落库、不广播：持久化与确认判定（ApplyConfirmPolicy）由宿主 / group 引擎负责。

package plan

import (
	"context"
	"errors"
	"fmt"

	"github.com/linkxzhou/SimpleBase/gocloudagent/agentcore"
)

// ErrNoPlan 表示模型没有产出可用的 plan 工具调用（提取失败或修复重试后仍校验失败）。
var ErrNoPlan = errors.New("planner: model did not produce a valid plan tool call")

// Planner 把一次用户消息变成结构化决策。
type Planner struct {
	// LLM 是模型入口（gateway / 直连客户端）。
	LLM agentcore.ChatClient
	// Scope / Model 透传给 ChatClient。
	Scope string
	Model string
	// MaxTokens 单次调用输出上限；0 取 2048（8 个任务 × 2000 字留余量）。
	MaxTokens int
	// Temperature；0 取 0.2。
	Temperature float64
	// MaxRepair 是校验失败后的修复重试次数；0 取 1，负数关闭。
	MaxRepair int
}

const (
	plannerDefaultMaxTokens = 2048
	plannerDefaultTemp      = 0.2
)

// Decide 产出一份通过 Validate 的决策。
func (p *Planner) Decide(ctx context.Context, req Request) (Decision, error) {
	if p == nil || p.LLM == nil {
		return Decision{}, errors.New("planner: LLM client is nil")
	}
	maxTokens := p.MaxTokens
	if maxTokens <= 0 {
		maxTokens = plannerDefaultMaxTokens
	}
	temp := p.Temperature
	if temp == 0 {
		temp = plannerDefaultTemp
	}

	// 文本工具协议（原生 tools 不可用时）：重建消息并去掉 Tools。
	msgs := BuildPrompt(req, false)
	tools := []agentcore.ToolSpec{PlanToolSpec()}
	call := func() (agentcore.ChatResponse, error) {
		return p.LLM.Chat(ctx, p.Scope, agentcore.ChatRequest{
			Model: p.Model, Messages: msgs, Tools: tools,
			MaxTokens: &maxTokens, Temperature: &temp,
		})
	}

	resp, err := call()
	if err != nil && len(tools) > 0 && agentcore.ToolsUnsupported(err) {
		tools = nil
		msgs = BuildPrompt(req, true)
		resp, err = call()
	}
	if err != nil {
		return Decision{}, err
	}

	repairs := p.MaxRepair
	if repairs == 0 {
		repairs = 1
	} else if repairs < 0 {
		repairs = 0
	}
	var lastErr error
	for attempt := 0; attempt <= repairs; attempt++ {
		d, derr := decisionFrom(resp)
		if derr == nil {
			d = Normalize(d, req.Constraints)
			if verr := Validate(d, req.Constraints); verr != nil {
				derr = verr
			} else {
				return d, nil
			}
		}
		lastErr = derr
		if attempt == repairs {
			break
		}
		// 修复重试：把失败输出作为引用回给模型，要求重新调用 plan。
		msgs = append(msgs, agentcore.ChatMessage{
			Role: "user",
			Content: "你上一次的输出是：\n" + agentcore.Truncate(rawOutput(resp), 4000) +
				"\n但它无效：" + lastErr.Error() +
				"。请重新调用 plan 工具输出修正后的完整计划（action=create），不要解释。",
		})
		resp, err = call()
		if err != nil {
			return Decision{}, err
		}
	}
	return Decision{}, fmt.Errorf("%w: %v", ErrNoPlan, lastErr)
}

// decisionFrom 从一次响应中提取 plan 工具调用的参数。
func decisionFrom(resp agentcore.ChatResponse) (Decision, error) {
	for _, tc := range resp.ToolCalls {
		if tc.Name == ToolPlan {
			return ParseDecision(tc.Arguments)
		}
	}
	if name, args, ok := agentcore.ExtractToolCall(resp.Content); ok && name == ToolPlan {
		return ParseDecision(args)
	}
	return Decision{}, fmt.Errorf("%w: no plan tool call in response", ErrNoPlan)
}

// rawOutput 把响应还原为可引用的文本（修复重试用）。
func rawOutput(resp agentcore.ChatResponse) string {
	for _, tc := range resp.ToolCalls {
		if tc.Name == ToolPlan {
			return tc.Arguments
		}
	}
	if resp.Content != "" {
		return resp.Content
	}
	return "(empty response)"
}
