package agentbridge

import "github.com/linkxzhou/SimpleBase/gocloudagent/cloudagent"

type (
	RunRequest           = cloudagent.RunRequest
	RunResult            = cloudagent.RunResult
	Event                = cloudagent.Event
	ChatRequest          = cloudagent.ChatRequest
	ChatResponse         = cloudagent.ChatResponse
	TokenStream          = cloudagent.TokenStream
	ToolDecision         = cloudagent.ToolDecision
	tracer               = cloudagent.Tracer
	ConfirmationPreparer = cloudagent.ConfirmationPreparer
)

func hashSummary(s string) string { return cloudagent.HashSummary(s) }
