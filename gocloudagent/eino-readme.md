# Eino 指南（SimpleBase Cloud Agent）

本文说明 CloudWeGo [Eino](https://github.com/cloudwego/eino) 在本仓库里的位置、v0.9.19 的公开 API，以及 `internal/gocloudagent` 实际怎么用它。API 名称和签名以 `github.com/cloudwego/eino v0.9.19` 源码为准（commit `9d983b36a5112a1c233056b1a099825298fafb8f`）。文中带 `package main` 的示例已在临时模块里用该版本 `go run` 跑通，临时目录没有提交。

Eino 是 Go 的 LLM 应用框架：用统一的 schema 描述消息和工具，用 component 接口接模型和工具，用 compose 把组件编成 Chain / Graph / Workflow，再用 `flow/agent` 或 `adk` 跑 ReAct 与多 Agent。SimpleBase 的云 Agent **运行时只走 ADK 的 `ChatModelAgent` + `Runner`**，工具用 `components/tool/utils.InferTool` 生成，模型用自研的 `gatewayChatModel` 接到 llmgateway，没有直接依赖 `eino-ext`。

## 目录

1. [阅读范围与仓库约定](#1-阅读范围与仓库约定)
2. [概览与核心概念](#2-概览与核心概念)
3. [安装与版本](#3-安装与版本)
4. [schema](#4-schema)
5. [Component 接口](#5-component-接口)
6. [工具定义](#6-工具定义)
7. [compose 编排](#7-compose-编排)
8. [ReAct 与 Host 多 Agent](#8-react-与-host-多-agent)
9. [ADK](#9-adk)
10. [Callbacks](#10-callbacks)
11. [中断、恢复与 Checkpoint](#11-中断恢复与-checkpoint)
12. [流式输出与 tool call 增量](#12-流式输出与-tool-call-增量)
13. [错误、超时与重试](#13-错误超时与重试)
14. [测试](#14-测试)
15. [在 SimpleBase gocloudagent 中如何使用](#15-在-simplebase-gocloudagent-中如何使用)
16. [最佳实践与常见问题](#16-最佳实践与常见问题)
17. [参考链接](#17-参考链接)

## 1. 阅读范围与仓库约定

本文覆盖的包：

| 包 | 作用 |
| --- | --- |
| `schema` | `Message`、`ToolInfo`、`StreamReader`/`StreamWriter`、`Document` |
| `components/model` | `BaseChatModel`、`ChatModel`、`ToolCallingChatModel` |
| `components/tool` | `InvokableTool`、`StreamableTool`，以及 Enhanced 变体 |
| `components/tool/utils` | `InferTool`、`NewTool`、流式工具、错误包装 |
| `components/prompt` | `ChatTemplate`、`FromMessages` |
| `components/embedding`、`indexer`、`retriever`、`document` | 向量与文档组件接口 |
| `compose` | Chain、Graph、Workflow、ToolsNode、State、Lambda、分支、并行、Checkpoint |
| `callbacks`、`utils/callbacks` | 组件生命周期回调 |
| `flow/agent/react` | ReAct Agent |
| `flow/agent/multiagent/host` | Host 把任务交给 Specialist |
| `adk` | `ChatModelAgent`、`Runner`、`AgentEvent`、子 Agent、interrupt/resume |

本仓库 `go.mod` 只 require `github.com/cloudwego/eino v0.9.19`。`eino-ext` 与 `eino-examples` 是独立仓库，第 3 节和第 17 节按源码列出，SimpleBase 运行时不 import 它们。

改 `internal/gocloudagent` 时同时遵守 [`internal/AGENTS.md`](../AGENTS.md)：云 Agent 只读访问数据，或在 `skills_cli` 打开时经 CLI 调既有 HTTP API；沙盒工具经 `Sandbox` 接口。禁止直接写 DuckLake / S3 / 系统库，禁止向沙盒注入 SimpleBase 凭据。日志和审计不得输出 LLM 正文。

## 2. 概览与核心概念

一次调用可以看成四层：

```text
用户消息 []*schema.Message
    │
    ▼
ChatTemplate / Instruction          把变量渲染成消息
    │
    ▼
ChatModel.Generate / Stream         产出文本或 ToolCall
    │
    ▼
ToolsNode                           按 ToolCall 执行 InvokableTool / StreamableTool
    │
    ▼
把 schema.ToolMessage 追加回上下文，再调模型，直到没有工具调用
```

编排层把上面的步骤变成可编译的图。编译结果是 `compose.Runnable[I, O]`，四种调用语义：

| 方法 | 输入 | 输出 | 典型用途 |
| --- | --- | --- | --- |
| `Invoke` | `I` | `O` | 一次性请求 |
| `Stream` | `I` | `*schema.StreamReader[O]` | 边生成边读 |
| `Collect` | `*schema.StreamReader[I]` | `O` | 先收完输入流再得到一个值 |
| `Transform` | `*schema.StreamReader[I]` | `*schema.StreamReader[O]` | 流进流出 |

节点只实现其中一种时，框架会用另外三种去适配。字符串流在收集时拼接；整数等标量取最后一个值；`*schema.Message` 走 `schema.ConcatMessages`。

Agent 有两套入口，不要混用配置类型：

- `flow/agent/react.Agent`：一个 ChatModel 节点加一个 Tools 节点，在图里循环。配置是 `react.AgentConfig`，调用是 `Generate` / `Stream`。
- `adk.ChatModelAgent`：ADK 的对话 Agent。配置是 `adk.ChatModelAgentConfig`，由 `adk.Runner` 跑，事件类型是 `*adk.AgentEvent`。SimpleBase 用的是这一套。

`components.Typer`（`GetType() string`）给调试界面一个类型名。`components.Checker`（`IsCallbacksEnabled() bool`）为 true 时，框架不再自动包一层 OnStart/OnEnd，由组件自己发回调。`DefaultChatTemplate` 两者都实现了。

## 3. 安装与版本

SimpleBase 锁定：

```go
require github.com/cloudwego/eino v0.9.19
```

模块自己的 `go` 行是 `go 1.18`，本仓库是 `go 1.25.0`。临时验证示例时：

```bash
mkdir /tmp/eino-doc && cd /tmp/eino-doc
go mod init example
go get github.com/cloudwego/eino@v0.9.19
go mod tidy
```

不要把示例模块提交进本仓库，也不要为了试 Eino 去改根 `go.mod`。

### 3.1 与 v0.9.21 的差异

撰写本文时 GitHub 上最新 tag 是 `v0.9.21`（`v0.9.19..v0.9.21` 共 19 个文件、约 +775/−39 行）。业务代码仍编译 v0.9.19。差异集中在：

- `schema.PromptTokenDetails` 与 `model.PromptTokenDetails` 增加 `CacheWriteTokens`。`ConcatMessages` 合并 usage 时对这个字段取更大值。v0.9.19 只有 `CachedTokens`。
- `compose` 的 checkpoint 在把流物化成值时，能识别工具流被取消（`toolStreamCheckpointError`），把已完成的工具结果留下、把被取消的 call id 记成 rerun，再转成 `CompositeInterrupt`。v0.9.19 的 `ToolsNode.Stream` 没有这层 tracker。
- `adk.StreamCanceledError` 在 v0.9.21 实现 `Is`，使它与 `internal/core.ErrStreamCanceled` 互相匹配。v0.9.19 只有 `Error() string`。
- 内存文件系统 backend 与若干 cancel 测试有小调整。`Chain` / `Graph` / `Workflow` / `ChatModelAgent` / `Runner` 的对外签名在这两版之间没有改名。

下文签名全部按 v0.9.19。升到 v0.9.21 之前要改 `go.mod`，那超出本文范围。

### 3.2 eino-ext 与 eino-examples

[eino-ext](https://github.com/cloudwego/eino-ext) 每个实现是独立 module，版本各自演进。本文核对的是 2026-10-13 的 `main`（`3603a39`）。SimpleBase 的模型不走这些包，云 Agent 的 `gatewayChatModel` 把 `schema.Message` 转成 llmgateway 的 `ChatRequest`。

常用 ChatModel 构造函数（签名来自上述 main）：

| 模块 | 构造函数 | 配置要点 |
| --- | --- | --- |
| `components/model/openai` | `NewChatModel(ctx, *ChatModelConfig) (*ChatModel, error)` | `APIKey`、`Model`、`BaseURL`；Azure 时 `ByAzure`、`APIVersion` |
| `components/model/ark` | `NewChatModel(ctx, *ChatModelConfig) (*ChatModel, error)` | `APIKey` 或 `AccessKey`+`SecretKey`，`Model` 是方舟 endpoint；默认 BaseURL `https://ark.cn-beijing.volces.com/api/v3` |
| `components/model/ollama` | `NewChatModel(ctx, *ChatModelConfig) (*ChatModel, error)` | `BaseURL`、`Model` |
| `components/model/deepseek` | `NewChatModel(ctx, *ChatModelConfig) (*ChatModel, error)` | `APIKey`、`Model`；默认 BaseURL `https://api.deepseek.com/` |
| `components/model/claude` | `NewChatModel(ctx, *Config) (*ChatModel, error)` | `APIKey`、`Model`、`MaxTokens`；可用 Bedrock / Vertex |
| `components/model/gemini` | `NewChatModel(ctx, *Config) (*ChatModel, error)` | Gemini API |
| `components/model/qwen` | `NewChatModel(ctx, *ChatModelConfig) (*ChatModel, error)` | 通义 |

这些类型都声明实现 `model.ToolCallingChatModel`（同时满足 `model.ChatModel`）。另有 `agenticopenai`、`agenticark`、`agenticclaude`、`agenticgemini`、`agenticdeepseek`、`agenticqwen`，面向 `*schema.AgenticMessage`，构造函数多是 `New` 或 `NewChatModel`。SimpleBase 的消息类型是 `*schema.Message`，不要接到 Agentic 模型上。

常用工具（同样来自该 main，只列入口）：

| 模块 | 入口 |
| --- | --- |
| `components/tool/httprequest` | `NewToolKit(ctx, *Config) ([]tool.BaseTool, error)`；GET/POST/PUT/DELETE 各有 `NewTool` |
| `components/tool/duckduckgo` | `NewTool`；v2 是 `NewTextSearchTool` |
| `components/tool/googlesearch` | `NewTool` |
| `components/tool/wikipedia` | `NewTool` |
| `components/tool/bingsearch` | `NewTool` |
| `components/tool/sequentialthinking` | `NewTool() (tool.InvokableTool, error)` |
| `components/tool/browseruse` | `NewBrowserUseTool` |
| `components/tool/commandline` | `NewStrReplaceEditor`、`NewPyExecutor` |
| `components/tool/mcp` | MCP 工具适配 |

回调后端在 `eino-ext/callbacks/`：`cozeloop`、`langfuse`、`langsmith`、`apmplus`。本仓库云 Agent 没有挂这些 handler。

[eino-examples](https://github.com/cloudwego/eino-examples) 的根 `go.mod` 在本文核对时依赖 eino `v0.9.21`，个别子目录仍钉着更老的 beta。读示例时以目录结构为准，编译前看该目录自己的 `go.mod`。对照路径见第 17 节。

## 4. schema

### 4.1 Message

`schema.RoleType`：

```go
const (
    Assistant RoleType = "assistant"
    User      RoleType = "user"
    System    RoleType = "system"
    Tool      RoleType = "tool"
)
```

`schema.Message` 的字段（`schema/message.go`）：

| 字段 | 用途 |
| --- | --- |
| `Role RoleType` | 角色 |
| `Content string` | 用户文本输入、模型文本输出 |
| `MultiContent []ChatMessagePart` | 已废弃，改用下面两个字段 |
| `UserInputMultiContent []MessageInputPart` | 用户多模态输入 |
| `AssistantGenMultiContent []MessageOutputPart` | 模型多模态输出 |
| `Name string` | 可选名字；`ConcatMessages` 要求同名才能合并 |
| `ToolCalls []ToolCall` | assistant 发起的工具调用 |
| `ToolCallID string` | tool 消息对应的 call id |
| `ToolName string` | tool 消息的工具名 |
| `ResponseMeta *ResponseMeta` | `FinishReason`、`Usage`、`LogProbs` |
| `ReasoningContent string` | 推理文本 |
| `Extra map[string]any` | 实现自用。gocloudagent 用 `Extra["delta"]=true` 标记增量 tool call |

构造函数：

```go
func SystemMessage(content string) *Message
func UserMessage(content string) *Message
func AssistantMessage(content string, toolCalls []ToolCall) *Message
func ToolMessage(content string, toolCallID string, opts ...ToolMessageOption) *Message
func WithToolName(name string) ToolMessageOption
```

`ToolCall`：

```go
type ToolCall struct {
    Index    *int           `json:"index,omitempty"`
    ID       string         `json:"id"`
    Type     string         `json:"type"`
    Function FunctionCall   `json:"function"`
    Extra    map[string]any `json:"extra,omitempty"`
}

type FunctionCall struct {
    Name      string `json:"name,omitempty"`
    Arguments string `json:"arguments,omitempty"`
}
```

`Index` 是流式合并的键。多个 chunk 的 `Index` 相同，`ConcatMessages` 才把 `ID`、`Type`、`Function.Name` 取首个非空值，把 `Arguments` 按顺序拼接。`Index == nil` 的 chunk 各自成为独立 `ToolCall`，空 name 的片段会让 `ToolsNode` 去找一个不存在的工具。这是 gocloudagent 在 `gatewayChatModel.Stream` 里写 `Index: &idx` 的原因。

`ConcatMessages` 的其他规则：

- 角色、`Name`、`ToolCallID`、`ToolName` 必须一致（空值可以后补），否则返回 error。
- `Content`、`ReasoningContent` 按出现顺序拼接。
- `ResponseMeta.FinishReason` 保留最后一个非空值。
- `Usage` 的 token 数取各 chunk 中的较大值，v0.9.19 覆盖 `PromptTokens`、`CompletionTokens`、`TotalTokens`、`CachedTokens`、`ReasoningTokens`。
- chunk 为 nil 会报 `unexpected nil chunk`。

`ResponseMeta.FinishReason` 的常见取值由模型实现决定，源码注释列举 `stop`、`length`、`tool_calls`、`content_filter`、`null`。

多模态 part 的类型常量：`ChatMessagePartTypeText`、`ImageURL`、`AudioURL`、`VideoURL`、`FileURL`、`Reasoning`、`ToolSearchResult`。输入用 `MessageInputImage` 等，输出用 `MessageOutputImage` 等。`ChatMessagePart` / `ChatMessageImageURL` 已标记 Deprecated。

### 4.2 模板

`MessagesTemplate`：

```go
type MessagesTemplate interface {
    Format(ctx context.Context, vs map[string]any, formatType FormatType) ([]*Message, error)
}
```

`FormatType`：`FString`（`{name}`，pyfmt）、`GoTemplate`（`{{.name}}`）、`Jinja2`。

```go
func MessagesPlaceholder(key string, optional bool) MessagesTemplate
```

`optional == false` 且 map 里没有该 key 时，`Format` 返回 `message placeholder format: %s not found`。值的动态类型必须是 `[]*schema.Message`。

`prompt.FromMessages` 把一组模板收成 `ChatTemplate`，见第 5 节。`Message` 自己也实现 `MessagesTemplate`，`Format` 会按 `formatType` 渲染 `Content`。

### 4.3 ToolInfo 与参数

```go
type ToolInfo struct {
    Name  string
    Desc  string
    Extra map[string]any
    *ParamsOneOf
}

func NewParamsOneOfByParams(params map[string]*ParameterInfo) *ParamsOneOf
func NewParamsOneOfByJSONSchema(s *jsonschema.Schema) *ParamsOneOf
func (p *ParamsOneOf) ToJSONSchema() (*jsonschema.Schema, error)
```

`ParamsOneOf` 只能走一种表示。`nil` 表示工具没有参数。`ParameterInfo`：

```go
type ParameterInfo struct {
    Type      DataType
    ElemInfo  *ParameterInfo
    SubParams map[string]*ParameterInfo
    Desc      string
    Enum      []string
    Required  bool
}
```

`DataType`：`Object`、`Number`、`Integer`、`String`、`Array`、`Null`、`Boolean`。

`ToolChoice` 通过 `model.WithToolChoice` 传入：

| 常量 | 值 | 含义 |
| --- | --- | --- |
| `ToolChoiceForbidden` | `forbidden` | 不调用工具 |
| `ToolChoiceAllowed` | `allowed` | 模型自己决定，有工具时的默认行为 |
| `ToolChoiceForced` | `forced` | 至少调用一个工具 |

### 4.4 StreamReader / StreamWriter

```go
func Pipe[T any](cap int) (*StreamReader[T], *StreamWriter[T])
func (sw *StreamWriter[T]) Send(chunk T, err error) (closed bool)
func (sw *StreamWriter[T]) Close()
func (sr *StreamReader[T]) Recv() (T, error)
func (sr *StreamReader[T]) Close()
func (sr *StreamReader[T]) Copy(n int) []*StreamReader[T]
func (sr *StreamReader[T]) SetAutomaticClose()
func StreamReaderFromArray[T any](arr []T) *StreamReader[T]
func MergeStreamReaders[T any](srs []*StreamReader[T]) *StreamReader[T]
func MergeNamedStreamReaders[T any](srs map[string]*StreamReader[T]) *StreamReader[T]
func StreamReaderWithConvert[T, D any](sr *StreamReader[T], convert func(T) (D, error), opts ...ConvertOption) *StreamReader[D]
func GetSourceName(err error) (string, bool)
```

约定：

- `Send` 的第二个返回值 `closed == true` 表示对端已经关掉，发送方应停止。
- 正常结束发送 `Close()`，接收方 `Recv` 得到 `io.EOF`。
- 接收方无论 `break` 还是 `return` 都要 `Close` 一次。`Copy(n)` 之后原 reader 不能再 `Recv`，要关掉的是 n 个副本。
- `schema.ErrNoValue` 只给 `StreamReaderWithConvert` 的转换函数用，表示丢掉这个元素。
- `schema.ErrRecvAfterClosed` 表示 `Close` 之后还在 `Recv`。
- `MergeNamedStreamReaders` 在某一路结束时，`Recv` 可能返回 `*schema.SourceEOF`，用 `GetSourceName` 取出名字。`WithErrWrapper`、`WithOnEOF` 是转换选项。

`init` 里已经注册 `ConcatMessages`、`ConcatMessageArray`、`ConcatToolResults`。自定义流元素用 `compose.RegisterStreamChunkConcatFunc[T]`。

### 4.5 Document

```go
type Document struct {
    ID       string
    Content  string
    MetaData map[string]any
}
```

元数据不要手写内部 key，用方法：`WithScore` / `Score`、`WithSubIndexes` / `SubIndexes`、`WithExtraInfo` / `ExtraInfo`、`WithDSLInfo` / `DSLInfo`，以及 dense / sparse vector 的对应方法。Retriever 写入的分数是 `WithScoreThreshold` 的过滤依据。

### 4.6 可运行示例

下面的程序覆盖消息构造、FString 模板、按 Index 拼接、缺 Index 时拆成两次调用，以及 `Pipe`。与本文其余 `package main` 示例一样，依赖 `github.com/cloudwego/eino v0.9.19`。

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "io"
    "strings"

    "github.com/cloudwego/eino/components/prompt"
    "github.com/cloudwego/eino/schema"
)

func main() {
    ctx := context.Background()
    call := schema.ToolCall{
        ID:   "call_1",
        Type: "function",
        Function: schema.FunctionCall{
            Name:      "list_databases",
            Arguments: `{}`,
        },
    }
    assistant := schema.AssistantMessage("", []schema.ToolCall{call})
    toolMsg := schema.ToolMessage(`{"databases":["shop"]}`, call.ID, schema.WithToolName(call.Function.Name))
    if assistant.ToolCalls[0].Function.Name != "list_databases" || toolMsg.Role != schema.Tool {
        panic("message constructors")
    }

    tpl := prompt.FromMessages(schema.FString,
        schema.SystemMessage("你是 {persona}"),
        schema.MessagesPlaceholder("history", false),
        schema.UserMessage("{query}"),
    )
    msgs, err := tpl.Format(ctx, map[string]any{
        "persona": "SimpleBase 助手",
        "history": []*schema.Message{
            schema.UserMessage("hi"),
            schema.AssistantMessage("hello", nil),
        },
        "query": "现在有哪些库？",
    })
    if err != nil || len(msgs) != 4 || !strings.Contains(msgs[0].Content, "SimpleBase") {
        panic(fmt.Sprint(err, len(msgs)))
    }

    idx := 0
    chunks := []*schema.Message{
        {Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
            Index: &idx, ID: "c1", Type: "function",
            Function: schema.FunctionCall{Name: "echo"},
        }}},
        {Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
            Index: &idx,
            Function: schema.FunctionCall{Arguments: `{"text":`},
        }}},
        {Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
            Index: &idx,
            Function: schema.FunctionCall{Arguments: `"hi"}`},
        }}},
    }
    merged, err := schema.ConcatMessages(chunks)
    if err != nil {
        panic(err)
    }
    if len(merged.ToolCalls) != 1 || merged.ToolCalls[0].Function.Arguments != `{"text":"hi"}` {
        panic(merged.ToolCalls)
    }

    bare, err := schema.ConcatMessages([]*schema.Message{
        {Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
            ID: "c1", Type: "function", Function: schema.FunctionCall{Name: "echo"},
        }}},
        {Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
            Function: schema.FunctionCall{Arguments: `{"text":"hi"}`},
        }}},
    })
    if err != nil || len(bare.ToolCalls) != 2 {
        panic("nil Index 会变成两次独立调用")
    }

    sr, sw := schema.Pipe[*schema.Message](2)
    go func() {
        defer sw.Close()
        sw.Send(schema.AssistantMessage("hel", nil), nil)
        sw.Send(schema.AssistantMessage("lo", nil), nil)
    }()
    var got []*schema.Message
    for {
        chunk, err := sr.Recv()
        if errors.Is(err, io.EOF) {
            break
        }
        if err != nil {
            panic(err)
        }
        got = append(got, chunk)
    }
    sr.Close()
    joined, err := schema.ConcatMessages(got)
    if err != nil || joined.Content != "hello" {
        panic(joined)
    }
}
```

## 5. Component 接口

组件通过 option 传运行时参数。每个包都有 `WrapImplSpecificOptFn` / `GetImplSpecificOptions`，实现方用它读取自己的私有 option。通用字段用 `GetCommonOptions`。

### 5.1 model

```go
type BaseModel[M messageType] interface {
    Generate(ctx context.Context, input []M, opts ...Option) (M, error)
    Stream(ctx context.Context, input []M, opts ...Option) (*schema.StreamReader[M], error)
}

type BaseChatModel = BaseModel[*schema.Message]
type AgenticModel = BaseModel[*schema.AgenticMessage]

type ChatModel interface {
    BaseChatModel
    BindTools(tools []*schema.ToolInfo) error
}

type ToolCallingChatModel interface {
    BaseChatModel
    WithTools(tools []*schema.ToolInfo) (ToolCallingChatModel, error)
}
```

`BindTools` 把工具记在原对象上。`WithTools` 返回一个带工具的新实例，原对象保持不变。ADK 的 `ChatModelAgent` 把工具放进 `model.WithTools` **调用 option**，模型要在 `Generate`/`Stream` 里用 `model.GetCommonOptions` 读到 `Options.Tools`。只实现 `WithTools` 方法、却忽略 option 的模型，在 ADK 里看不到工具。`react.Agent` 会调用 `ToolCallingChatModel.WithTools`。gocloudagent 的 `gatewayChatModel` 两个都做了：`WithTools` 复制一份并保存 `bound`，`Generate` 再与 option 里的工具合并，option 优先。

`model.Options` 里和调用相关的字段：`Temperature`、`Model`、`TopP`、`Tools`、`DeferredTools`、`ToolSearchTool`、`MaxTokens`、`Stop`，以及 tool choice。对应函数：

```go
func WithTemperature(temperature float32) Option
func WithMaxTokens(maxTokens int) Option
func WithModel(name string) Option
func WithTopP(topP float32) Option
func WithStop(stop []string) Option
func WithTools(tools []*schema.ToolInfo) Option
func WithDeferredTools(tools []*schema.ToolInfo) Option
func WithToolSearchTool(tool *schema.ToolInfo) Option
func WithToolChoice(toolChoice schema.ToolChoice, allowedToolNames ...string) Option
func GetCommonOptions(base *Options, opts ...Option) *Options
```

回调结构在 `components/model/callback_extra.go`：`CallbackInput` 含 `Messages`、`Tools`、`ToolChoice`、`Config`；`CallbackOutput` 含 `Message`、`TokenUsage`。用 `model.ConvCallbackInput` / `ConvCallbackOutput` 从 `callbacks.CallbackInput` 转出来。

### 5.2 tool

```go
type BaseTool interface {
    Info(ctx context.Context) (*schema.ToolInfo, error)
}

type InvokableTool interface {
    BaseTool
    InvokableRun(ctx context.Context, argumentsInJSON string, opts ...Option) (string, error)
}

type StreamableTool interface {
    BaseTool
    StreamableRun(ctx context.Context, argumentsInJSON string, opts ...Option) (*schema.StreamReader[string], error)
}

type EnhancedInvokableTool interface {
    BaseTool
    InvokableRun(ctx context.Context, toolArgument *schema.ToolArgument, opts ...Option) (*schema.ToolResult, error)
}

type EnhancedStreamableTool interface {
    BaseTool
    StreamableRun(ctx context.Context, toolArgument *schema.ToolArgument, opts ...Option) (*schema.StreamReader[*schema.ToolResult], error)
}
```

`ToolsNode` 要求 `BaseTool` 至少实现 Invokable、Streamable、Enhanced 之一。同一个工具可以同时实现 Invokable 和 Streamable；节点在 `Invoke` 时走 Invokable，在 `Stream` 时优先走 Streamable。Enhanced 版本的返回值是 `*schema.ToolResult`，可以带图片、音频等 `ToolOutputPart`。gocloudagent 的工具全部是 Invokable，返回 JSON 字符串。

中断函数在 `components/tool/interrupt.go`，见第 11 节：

```go
func Interrupt(ctx context.Context, info any) error
func StatefulInterrupt(ctx context.Context, info any, state any) error
func GetInterruptState[T any](ctx context.Context) (wasInterrupted bool, hasState bool, state T)
func GetResumeContext[T any](ctx context.Context) (isResumeTarget bool, hasData bool, data T)
```

### 5.3 prompt

```go
type ChatTemplate interface {
    Format(ctx context.Context, vs map[string]any, opts ...Option) ([]*schema.Message, error)
}

func FromMessages(formatType schema.FormatType, templates ...schema.MessagesTemplate) *DefaultChatTemplate
```

`DefaultChatTemplate.GetType()` 返回固定类型名，`IsCallbacksEnabled()` 为 true。Agentic 对应 `AgenticChatTemplate` 与 `FromAgenticMessages`。

### 5.4 embedding、indexer、retriever、document

```go
type Embedder interface {
    EmbedStrings(ctx context.Context, texts []string, opts ...Option) ([][]float64, error)
}

type Indexer interface {
    Store(ctx context.Context, docs []*schema.Document, opts ...Option) (ids []string, err error)
}

type Retriever interface {
    Retrieve(ctx context.Context, query string, opts ...Option) ([]*schema.Document, error)
}

type Loader interface {
    Load(ctx context.Context, src Source, opts ...LoaderOption) ([]*schema.Document, error)
}

type Transformer interface {
    Transform(ctx context.Context, src []*schema.Document, opts ...TransformerOption) ([]*schema.Document, error)
}
```

`document.Source` 只有 `URI`。`parser.Parser` 是 `Parse(ctx, io.Reader, ...Option) ([]*schema.Document, error)`。内置 `parser.TextParser`，以及按扩展名分发的 `parser.NewExtParser`。

常用 option：

- embedding：`embedding.WithModel`
- indexer：`indexer.WithIndex`、`WithSubIndexes`、`WithEmbedding`
- retriever：`retriever.WithIndex`、`WithSubIndex`、`WithTopK`、`WithScoreThreshold`、`WithEmbedding`、`WithDSLInfo`
- parser：`parser.WithURI`、`parser.WithExtraMeta`

`flow/retriever` 在接口之上提供 `parent`（父文档召回）、`multiquery`、`router`。`flow/indexer/parent` 是父文档索引。gocloudagent 没有接这些组件；数据库查询走只读 SQL 工具。

Chain / Graph / Workflow 都有 `AppendEmbedding`、`AppendRetriever`、`AppendLoader`、`AppendIndexer`、`AppendDocumentTransformer`（Graph 上叫 `Add*Node`）。

## 6. 工具定义

`components/tool/utils` 有两条路：从结构体推断 schema，或自己写 `ToolInfo`。

### 6.1 InferTool / NewTool

```go
type InvokeFunc[T, D any] func(ctx context.Context, input T) (output D, err error)

func InferTool[T, D any](toolName, toolDesc string, i InvokeFunc[T, D], opts ...Option) (tool.InvokableTool, error)
func InferOptionableTool[T, D any](toolName, toolDesc string, i OptionableInvokeFunc[T, D], opts ...Option) (tool.InvokableTool, error)
func NewTool[T, D any](desc *schema.ToolInfo, i InvokeFunc[T, D], opts ...Option) tool.InvokableTool
func GoStruct2ParamsOneOf[T any](opts ...Option) (*schema.ParamsOneOf, error)
func GoStruct2ToolInfo[T any](toolName, toolDesc string, opts ...Option) (*schema.ToolInfo, error)
```

`InferTool` 用 `eino-contrib/jsonschema` 反射 `T`，得到 `ParamsOneOf` 的 JSON Schema 形式，再交给 `NewTool`。调用时把模型给的 JSON 解成 `T`，把返回值 `D` 再编码成字符串。`D` 是 `string` 时，结果就是这个字符串本身（gocloudagent 的工具都返回已经 `json.Marshal` 过的字符串）。

官方 tag 约定写在 `components/tool/utils/doc.go`：

```go
type Input struct {
    Query    string `json:"query" jsonschema:"required" jsonschema_description:"The search query"`
    MaxItems int    `json:"max_items" jsonschema_description:"Maximum results to return"`
}
```

`json` 决定模型看到的参数名。`jsonschema:"required"` 标必填。描述放在 `jsonschema_description`，不要写成 `jsonschema:"required,description=..."` 这种带逗号的形式，解析会把逗号当成分隔符。`WithSchemaModifier` 可以在反射后改 schema。

`NewTool` 不检查 `ToolInfo` 和 `T` 的字段是否一致，两边要自己对齐。

流式：

```go
func InferStreamTool[T, D any](toolName, toolDesc string, s StreamFunc[T, D], opts ...Option) (tool.StreamableTool, error)
func NewStreamTool[T, D any](desc *schema.ToolInfo, s StreamFunc[T, D], opts ...Option) tool.StreamableTool
```

`StreamFunc` 返回 `*schema.StreamReader[D]`。`D` 为 `string` 时，`StreamableRun` 的读者直接收到这些字符串。Enhanced 版本（`InferEnhancedTool`、`NewEnhancedTool`、`InferEnhancedStreamTool`、`NewEnhancedStreamTool`）返回 `*schema.ToolResult`。

错误可以变成工具文本，让模型看到失败原因并继续：

```go
func WrapInvokableToolWithErrorHandler(t tool.InvokableTool, h ErrorHandler) tool.InvokableTool
func WrapStreamableToolWithErrorHandler(t tool.StreamableTool, h ErrorHandler) tool.StreamableTool
type ErrorHandler func(context.Context, error) string
```

gocloudagent 没有用这个 helper，而是自己的 `errorTolerantTool`：业务错误编成 `{"error","is_error"}`，`context.Canceled` 和 `context.DeadlineExceeded` 原样返回，用来停掉整轮。

### 6.2 手写 ToolInfo

适合参数是动态拼出来的，或者需要 `Enum`、嵌套 `SubParams`：

```go
info := &schema.ToolInfo{
    Name: "add",
    Desc: "两数相加",
    ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
        "a": {Type: schema.Integer, Desc: "左操作数", Required: true},
        "b": {Type: schema.Integer, Desc: "右操作数", Required: true},
    }),
}
```

### 6.3 ToolsNode

```go
type ToolsNodeConfig struct {
    Tools                []tool.BaseTool
    ToolAliases          map[string]ToolAliasConfig
    UnknownToolsHandler  func(ctx context.Context, name, input string) (string, error)
    ExecuteSequentially  bool
    ToolArgumentsHandler func(ctx context.Context, name, arguments string) (string, error)
    ToolCallMiddlewares  []ToolMiddleware
}

func NewToolNode(ctx context.Context, conf *ToolsNodeConfig) (*ToolsNode, error)
func (tn *ToolsNode) Invoke(ctx context.Context, input *schema.Message, opts ...ToolsNodeOption) ([]*schema.Message, error)
func (tn *ToolsNode) Stream(ctx context.Context, input *schema.Message, opts ...ToolsNodeOption) (*schema.StreamReader[[]*schema.Message], error)
func GetToolCallID(ctx context.Context) string
```

行为：

- 输入必须是带 `ToolCalls` 的 assistant 消息。输出是一组 `Role == tool` 的消息，`ToolCallID` 与调用对应。
- `ExecuteSequentially == false`（默认）时并行执行。有共享可变状态或需要稳定顺序时设为 true。
- 模型编造了未注册的工具名：未设置 `UnknownToolsHandler` 时返回错误；设置了则用 handler 的字符串当作该次调用的结果。
- 参数处理顺序：`ToolAliases` 的 `ArgumentsAliases` 改名，然后 `ToolArgumentsHandler`，然后真正执行。
- 工具函数里用 `compose.GetToolCallID(ctx)` 取本次 call id。不在工具调用栈上时返回空字符串。gocloudagent 的 CLI 工具用它关联确认框和 `callGate`。
- 调用期可以 `WithToolOption`、`WithToolList` 替换工具列表。

`ToolMiddleware` 可以分别包 Invokable、Streamable、Enhanced。`react` 用它把工具结果旁路收集出来。

### 6.4 可运行示例

```go
package main

import (
    "context"
    "errors"
    "strings"

    "github.com/cloudwego/eino/components/tool"
    "github.com/cloudwego/eino/components/tool/utils"
    "github.com/cloudwego/eino/compose"
    "github.com/cloudwego/eino/schema"
)

type echoIn struct {
    Text string `json:"text" jsonschema:"required" jsonschema_description:"要回显的文本"`
}

func main() {
    ctx := context.Background()
    echo, err := utils.InferTool("echo", "回显文本", func(_ context.Context, in echoIn) (string, error) {
        if in.Text == "" {
            return "", errors.New("text required")
        }
        return "echo:" + in.Text, nil
    })
    if err != nil {
        panic(err)
    }
    out, err := echo.InvokableRun(ctx, `{"text":"hi"}`)
    if err != nil || out != "echo:hi" {
        panic(out)
    }

    manual := utils.NewTool(&schema.ToolInfo{
        Name: "add",
        Desc: "两数相加",
        ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
            "a": {Type: schema.Integer, Required: true},
            "b": {Type: schema.Integer, Required: true},
        }),
    }, func(_ context.Context, in struct {
        A int `json:"a"`
        B int `json:"b"`
    }) (int, error) {
        return in.A + in.B, nil
    })
    sum, err := manual.InvokableRun(ctx, `{"a":2,"b":3}`)
    if err != nil || sum != "5" {
        panic(sum)
    }

    safe := utils.WrapInvokableToolWithErrorHandler(echo, func(_ context.Context, err error) string {
        return "ERR:" + err.Error()
    })
    text, err := safe.InvokableRun(ctx, `{"text":""}`)
    if err != nil || !strings.HasPrefix(text, "ERR:") {
        panic(text)
    }

    node, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
        Tools:               []tool.BaseTool{echo},
        ExecuteSequentially: true,
        UnknownToolsHandler: func(_ context.Context, name, _ string) (string, error) {
            return "unknown:" + name, nil
        },
    })
    if err != nil {
        panic(err)
    }
    msgs, err := node.Invoke(ctx, schema.AssistantMessage("", []schema.ToolCall{{
        ID: "c", Type: "function",
        Function: schema.FunctionCall{Name: "echo", Arguments: `{"text":"x"}`},
    }}))
    if err != nil || len(msgs) != 1 || msgs[0].Content != "echo:x" || msgs[0].Role != schema.Tool {
        panic(msgs)
    }
}
```

## 7. compose 编排

`compose.START` 的值是 `"start"`，`compose.END` 的值是 `"end"`。节点 key 不要占用这两个名字。

### 7.1 Lambda

```go
func InvokableLambda[I, O any](i InvokeWOOpt[I, O], opts ...LambdaOpt) *Lambda
func StreamableLambda[I, O any](s StreamWOOpt[I, O], opts ...LambdaOpt) *Lambda
func CollectableLambda[I, O any](c CollectWOOpt[I, O], opts ...LambdaOpt) *Lambda
func TransformableLambda[I, O any](t TransformWOOpts[I, O], opts ...LambdaOpt) *Lambda
func AnyLambda[I, O, TOption any](i Invoke[I, O, TOption], s Stream[I, O, TOption], c Collect[I, O, TOption], t Transform[I, O, TOption], opts ...LambdaOpt) (*Lambda, error)
func ToList[I any](opts ...LambdaOpt) *Lambda
func WithLambdaCallbackEnable(y bool) LambdaOpt
func WithLambdaType(t string) LambdaOpt
```

只提供 `StreamableLambda` 时，`Invoke` 会把流出的 chunk 按类型拼成一个值。字符串拼接，`*schema.Message` 走 `ConcatMessages`。

### 7.2 Chain

```go
func NewChain[I, O any](opts ...NewGraphOption) *Chain[I, O]
func (c *Chain[I, O]) AppendChatTemplate(node prompt.ChatTemplate, opts ...GraphAddNodeOpt) *Chain[I, O]
func (c *Chain[I, O]) AppendChatModel(node model.BaseChatModel, opts ...GraphAddNodeOpt) *Chain[I, O]
func (c *Chain[I, O]) AppendToolsNode(node *ToolsNode, opts ...GraphAddNodeOpt) *Chain[I, O]
func (c *Chain[I, O]) AppendLambda(node *Lambda, opts ...GraphAddNodeOpt) *Chain[I, O]
func (c *Chain[I, O]) AppendBranch(b *ChainBranch) *Chain[I, O]
func (c *Chain[I, O]) AppendParallel(p *Parallel) *Chain[I, O]
func (c *Chain[I, O]) AppendPassthrough(opts ...GraphAddNodeOpt) *Chain[I, O]
func (c *Chain[I, O]) AppendGraph(node AnyGraph, opts ...GraphAddNodeOpt) *Chain[I, O]
func (c *Chain[I, O]) Compile(ctx context.Context, opts ...GraphCompileOption) (Runnable[I, O], error)
```

Chain 的输入类型 `I` 必须是第一个节点的输入，输出类型 `O` 必须是最后一个节点的输出。中间节点的类型由框架顺着边检查。模板节点吃 `map[string]any`，模型节点吃 `[]*schema.Message`、吐 `*schema.Message`。

分支：

```go
func NewChainBranch[T any](cond GraphBranchCondition[T]) *ChainBranch
func NewChainMultiBranch[T any](cond GraphMultiBranchCondition[T]) *ChainBranch
func (cb *ChainBranch) AddLambda(key string, node *Lambda, opts ...GraphAddNodeOpt) *ChainBranch
func (cb *ChainBranch) AddChatModel(key string, node model.BaseChatModel, opts ...GraphAddNodeOpt) *ChainBranch
func (cb *ChainBranch) AddPassthrough(key string, opts ...GraphAddNodeOpt) *ChainBranch
```

`cond` 返回的字符串必须是 `Add*` 时用的 key。多选分支返回 `map[string]bool`。分支之后如果要汇合，官方 chain 示例在后面接 `AppendPassthrough()`。

并行：

```go
func NewParallel() *Parallel
func (p *Parallel) AddLambda(outputKey string, node *Lambda, opts ...GraphAddNodeOpt) *Parallel
```

每个分支的输入类型与并行节点的输入相同。并行节点的输出类型是 `map[string]any`，key 就是 `AddLambda` 的 `outputKey`。

节点 option：`WithNodeName`、`WithNodeKey`、`WithInputKey`、`WithOutputKey`、`WithStatePreHandler`、`WithStatePostHandler`、`WithStreamStatePreHandler`、`WithStreamStatePostHandler`。

### 7.3 Graph

```go
func NewGraph[I, O any](opts ...NewGraphOption) *Graph[I, O]
func (g *Graph[I, O]) AddEdge(startNode, endNode string) error
func (g *Graph[I, O]) Compile(ctx context.Context, opts ...GraphCompileOption) (Runnable[I, O], error)
```

`AddChatModelNode`、`AddLambdaNode`、`AddToolsNode`、`AddBranch` 等方法定义在内嵌的 `*graph` 上，签名返回 `error`。`AddBranch` 使用：

```go
func NewGraphBranch[T any](condition GraphBranchCondition[T], endNodes map[string]bool) *GraphBranch
func NewStreamGraphBranch[T any](condition StreamGraphBranchCondition[T], endNodes map[string]bool) *GraphBranch
func NewGraphMultiBranch[T any](condition GraphMultiBranchCondition[T], endNodes map[string]bool) *GraphBranch
```

`endNodes` 必须覆盖条件函数可能返回的每一个 key，并且这些 key 已经 `Add*Node`。流式条件函数收到的是 `*schema.StreamReader`，读完要负责关闭，否则上游块会泄漏。`react` 和 host multi-agent 的 `StreamToolCallChecker` 注释写明 handler 必须关闭 `modelOutput`。

编译 option：

```go
func WithMaxRunSteps(maxSteps int) GraphCompileOption
func WithGraphName(graphName string) GraphCompileOption
func WithNodeTriggerMode(triggerMode NodeTriggerMode) GraphCompileOption
func WithEagerExecution() GraphCompileOption
func WithInterruptBeforeNodes(nodes []string) GraphCompileOption
func WithInterruptAfterNodes(nodes []string) GraphCompileOption
func WithCheckPointStore(store CheckPointStore) GraphCompileOption
func WithSerializer(serializer Serializer) GraphCompileOption
```

`NodeTriggerMode`：`AnyPredecessor`（任一前驱完成就触发）、`AllPredecessor`（全部前驱完成）。有多条入边时要显式选择。运行期 `compose.WithRuntimeMaxSteps` 可以覆盖编译期步数。

调用 option 把参数下发到某一类节点：`WithChatModelOption`、`WithToolsNodeOption`、`WithCallbacks`、`DesignateNode`。

### 7.4 Workflow

Workflow 仍编译成 `Runnable`，但边要声明字段映射，适合「结构体的几列分别进不同节点」。

```go
func NewWorkflow[I, O any](opts ...NewGraphOption) *Workflow[I, O]
func (wf *Workflow[I, O]) AddLambdaNode(key string, lambda *Lambda, opts ...GraphAddNodeOpt) *WorkflowNode
func (n *WorkflowNode) AddInput(fromNodeKey string, inputs ...*FieldMapping) *WorkflowNode
func (n *WorkflowNode) AddDependency(fromNodeKey string) *WorkflowNode
func (n *WorkflowNode) SetStaticValue(path FieldPath, value any) *WorkflowNode
func (wf *Workflow[I, O]) End() *WorkflowNode
func (wf *Workflow[I, O]) AddBranch(fromNodeKey string, branch *GraphBranch) *WorkflowBranch
func (wf *Workflow[I, O]) Compile(ctx context.Context, opts ...GraphCompileOption) (Runnable[I, O], error)
```

`AddInput` 同时建立控制依赖和数据依赖。不传 `FieldMapping` 时，前驱的整个输出作为当前节点的输入。字段映射：

```go
func MapFields(from, to string) *FieldMapping
func MapFieldPaths(fromFieldPath, toFieldPath FieldPath) *FieldMapping
func ToField(to string, opts ...FieldMappingOption) *FieldMapping
func FromField(from string) *FieldMapping
type FieldPath []string
```

被映射的字段必须导出，否则反射读不到。`eino-examples/compose/workflow/2_field_mapping` 把 `Message.Content` 映射到另一个结构体的 `FullStr`。`5_static_values` 演示 `SetStaticValue`，`4_control_only_branch` 演示只有控制依赖、没有数据依赖。

### 7.5 State

```go
type GenLocalState[S any] func(ctx context.Context) (state S)
func WithGenLocalState[S any](gls GenLocalState[S]) NewGraphOption

type StatePreHandler[I, S any] func(ctx context.Context, in I, state S) (I, error)
type StatePostHandler[O, S any] func(ctx context.Context, out O, state S) (O, error)

func ProcessState[S any](ctx context.Context, handler func(context.Context, S) error) error
```

`S` 在一次 `Invoke`/`Stream` 内共享。handler 里就地修改指针字段即可，`ProcessState` 持有锁。要 checkpoint 的状态类型必须 `schema.Register[T]()`，并且只有导出字段会被 gob 编码。函数、channel、未导出字段不会进快照。

### 7.6 四种调用语义

对一个只实现了 `StreamableLambda[string, string]`、把输入重复两遍的图：

| 调用 | 输入 | v0.9.19 实测结果 |
| --- | --- | --- |
| `Invoke(ctx, "z")` | 单值 | `"zz"`（框架收集流并拼接） |
| `Stream(ctx, "z")` | 单值 | 两个 chunk `"z"`、`"z"` |
| `Collect(ctx, StreamReaderFromArray([]string{"a","b"}))` | 流先拼成 `"ab"` 再送进节点 | `"abab"` |
| `Transform(ctx, StreamReaderFromArray([]string{"q"}))` | 流 | 读完得到 `"qq"` |

`Recv` 到 `io.EOF` 结束。业务错误不要用 `io.EOF` 表示。

### 7.7 可运行示例

示例依赖第 14 节的 `scriptModel`。把两段放进同一个 `package main` 即可运行。这里用完整文件，假模型内联了一份最小实现。

```go
package main

import (
    "context"
    "fmt"
    "strings"
    "sync"

    "github.com/cloudwego/eino/components/model"
    "github.com/cloudwego/eino/components/prompt"
    "github.com/cloudwego/eino/compose"
    "github.com/cloudwego/eino/schema"
)

type scriptModel struct {
    mu    sync.Mutex
    steps []*schema.Message
    n     int
}

func (m *scriptModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    if m.n >= len(m.steps) {
        return schema.AssistantMessage("done", nil), nil
    }
    msg := m.steps[m.n]
    m.n++
    return msg, nil
}

func (m *scriptModel) Stream(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
    msg, err := m.Generate(ctx, in, opts...)
    if err != nil {
        return nil, err
    }
    return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func (m *scriptModel) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
    return m, nil
}

func main() {
    ctx := context.Background()
    chain := compose.NewChain[map[string]any, *schema.Message]()
    chain.
        AppendChatTemplate(prompt.FromMessages(schema.GoTemplate,
            schema.SystemMessage("只回答一个词"),
            schema.UserMessage("{{.query}}"),
        )).
        AppendChatModel(&scriptModel{steps: []*schema.Message{schema.AssistantMessage("pong", nil)}})
    runnable, err := chain.Compile(ctx)
    if err != nil {
        panic(err)
    }
    msg, err := runnable.Invoke(ctx, map[string]any{"query": "ping"})
    if err != nil || msg.Content != "pong" {
        panic(msg)
    }

    g := compose.NewGraph[string, string]()
    if err := g.AddLambdaNode("upper", compose.InvokableLambda(func(_ context.Context, in string) (string, error) {
        return strings.ToUpper(in), nil
    })); err != nil {
        panic(err)
    }
    if err := g.AddLambdaNode("bang", compose.InvokableLambda(func(_ context.Context, in string) (string, error) {
        return in + "!", nil
    })); err != nil {
        panic(err)
    }
    _ = g.AddEdge(compose.START, "upper")
    _ = g.AddEdge("upper", "bang")
    _ = g.AddEdge("bang", compose.END)
    gr, err := g.Compile(ctx)
    if err != nil {
        panic(err)
    }
    got, err := gr.Invoke(ctx, "hi")
    if err != nil || got != "HI!" {
        panic(got)
    }

    bg := compose.NewGraph[string, string]()
    _ = bg.AddLambdaNode("a", compose.InvokableLambda(func(_ context.Context, in string) (string, error) {
        return "A:" + in, nil
    }))
    _ = bg.AddLambdaNode("b", compose.InvokableLambda(func(_ context.Context, in string) (string, error) {
        return "B:" + in, nil
    }))
    _ = bg.AddBranch(compose.START, compose.NewGraphBranch(func(_ context.Context, in string) (string, error) {
        if strings.HasPrefix(in, "a") {
            return "a", nil
        }
        return "b", nil
    }, map[string]bool{"a": true, "b": true}))
    _ = bg.AddEdge("a", compose.END)
    _ = bg.AddEdge("b", compose.END)
    br, err := bg.Compile(ctx)
    if err != nil {
        panic(err)
    }
    got, err = br.Invoke(ctx, "a-side")
    if err != nil || got != "A:a-side" {
        panic(got)
    }

    ch := compose.NewChain[string, map[string]any]()
    p := compose.NewParallel()
    p.AddLambda("len", compose.InvokableLambda(func(_ context.Context, in string) (int, error) {
        return len(in), nil
    }))
    p.AddLambda("upper", compose.InvokableLambda(func(_ context.Context, in string) (string, error) {
        return strings.ToUpper(in), nil
    }))
    pr, err := ch.AppendParallel(p).Compile(ctx)
    if err != nil {
        panic(err)
    }
    pm, err := pr.Invoke(ctx, "Ab")
    if err != nil || pm["len"] != 2 || pm["upper"] != "AB" {
        panic(fmt.Sprint(pm))
    }

    type counter struct {
        FullStr string
        SubStr  string
    }
    wf := compose.NewWorkflow[counter, map[string]any]()
    wf.AddLambdaNode("count", compose.InvokableLambda(func(_ context.Context, c counter) (int, error) {
        return strings.Count(c.FullStr, c.SubStr), nil
    })).AddInput(compose.START)
    wf.End().AddInput("count", compose.ToField("n"))
    wr, err := wf.Compile(ctx)
    if err != nil {
        panic(err)
    }
    wm, err := wr.Invoke(ctx, counter{FullStr: "banana", SubStr: "a"})
    if err != nil || wm["n"] != 3 {
        panic(fmt.Sprint(wm))
    }
}
```

## 8. ReAct 与 Host 多 Agent

### 8.1 react.Agent

```go
type MessageModifier func(ctx context.Context, input []*schema.Message) []*schema.Message

type AgentConfig struct {
    ToolCallingModel      model.ToolCallingChatModel
    Model                 model.ChatModel // Deprecated: 改用 ToolCallingModel
    ToolsConfig           compose.ToolsNodeConfig
    MessageModifier       MessageModifier
    MessageRewriter       MessageModifier
    MaxStep               int
    ToolReturnDirectly    map[string]struct{}
    StreamToolCallChecker func(ctx context.Context, modelOutput *schema.StreamReader[*schema.Message]) (bool, error)
    GraphName             string
    ModelNodeName         string
    ToolsNodeName         string
}

func NewPersonaModifier(persona string) MessageModifier
func NewAgent(ctx context.Context, config *AgentConfig) (*Agent, error)
func (r *Agent) Generate(ctx context.Context, input []*schema.Message, opts ...agent.AgentOption) (*schema.Message, error)
func (r *Agent) Stream(ctx context.Context, input []*schema.Message, opts ...agent.AgentOption) (*schema.StreamReader[*schema.Message], error)
```

图里固定两个业务节点，默认名 `ChatModel` 与 `Tools`（常量 `ModelNodeName`、`ToolsNodeName`，图名默认 `ReActAgent`）。模型产出 tool call 就进工具节点，工具结果写回 state 再回到模型；模型产出普通文本就结束。

`MaxStep` 是 pregel 步数上限，注释写默认是节点数 + 10，也就是 12。这和 ADK 的 `MaxIterations`（模型调用轮数，默认 20）不是同一个计数器。

`MessageRewriter` 先改 state 里累积的历史，`MessageModifier` 再改即将送给模型的那一份。压缩上下文用 Rewriter，因为修改会留到后续轮次。`NewPersonaModifier` 在输入前插入一条 system 消息。

`ToolReturnDirectly` 的 key 是工具名。该工具被调用后，Agent 把这个工具结果当作最终消息返回。同一轮多个命中时只返回第一个。

`StreamToolCallChecker` 只在 `Stream` 路径需要。默认实现看第一块有没有 tool call。Claude 经常先吐文本再吐 tool call，默认判断会把这种流当成「没有工具」直接结束。自定义 checker 必须 `Close` 传入的 reader。可以通过提示词约束模型在调用工具时不要先输出正文。

运行 option：

```go
func WithToolOptions(opts ...tool.Option) agent.AgentOption
func WithChatModelOptions(opts ...model.Option) agent.AgentOption
func WithToolList(tools ...tool.BaseTool) agent.AgentOption
func WithTools(ctx context.Context, tools ...tool.BaseTool) ([]agent.AgentOption, error)
func WithMessageFuture() (agent.AgentOption, MessageFuture)
```

`MessageFuture.GetMessages()` 与 `GetMessageStreams()` 能在 `Generate`/`Stream` 返回最终答案的同时，看到中间每一轮的模型和工具消息。

`react.BuildAgentCallback` 把 `utils/callbacks` 的 model/tool handler 收成一个 `callbacks.Handler`。

### 8.2 host.MultiAgent

```go
func NewMultiAgent(ctx context.Context, config *MultiAgentConfig) (*MultiAgent, error)

type MultiAgentConfig struct {
    Host                   Host
    Specialists            []*Specialist
    Name                   string
    HostNodeName           string
    StreamToolCallChecker  func(ctx context.Context, modelOutput *schema.StreamReader[*schema.Message]) (bool, error)
}

type Host struct {
    ToolCallingModel model.ToolCallingChatModel
    ChatModel        model.ChatModel // 旧字段
    SystemPrompt     string
}

type Specialist struct {
    AgentMeta
    ChatModel    model.BaseChatModel
    SystemPrompt string
    Invokable    compose.Invoke[[]*schema.Message, *schema.Message, agent.AgentOption]
    Streamable   compose.Stream[[]*schema.Message, *schema.Message, agent.AgentOption]
}

type AgentMeta struct {
    Name        string
    IntendedUse string
}
```

Host 模型把 Specialist 的 `Name` 当作工具名来调用。框架按 `ToolCall.Function.Name` 路由。一个 Specialist 要么提供 `ChatModel`，要么提供 `Invokable` / `Streamable`。Host 直接回答、不调用工具时，分支走到 `END`。只路由到一个 Specialist 时，该 Specialist 的输出就是最终消息；路由到多个时会先汇总。`StreamToolCallChecker` 的默认实现同样只看第一块，关闭流的义务相同。

`host.MultiAgentCallback` 只有 `OnHandOff(ctx, *HandOffInfo)`。`ConvertCallbackHandlers` 把它转成 `callbacks.Handler`。`WithAgentCallbacks` 是 `agent.AgentOption`。

这套图和 ADK 的 transfer 是两条产品线。源码在 Supervisor / `OutputKey` / `Exit` 上注明：全量上下文移交在经验上并不更好，更常见的做法是 `ChatModelAgent` 加 `NewAgentTool`，或 `adk/prebuilt/deep`。

### 8.3 可运行示例

```go
package main

import (
    "context"
    "sync"

    "github.com/cloudwego/eino/components/model"
    "github.com/cloudwego/eino/components/tool"
    "github.com/cloudwego/eino/components/tool/utils"
    "github.com/cloudwego/eino/compose"
    "github.com/cloudwego/eino/flow/agent"
    host "github.com/cloudwego/eino/flow/agent/multiagent/host"
    "github.com/cloudwego/eino/flow/agent/react"
    "github.com/cloudwego/eino/schema"
)

type scriptModel struct {
    mu    sync.Mutex
    steps []*schema.Message
    n     int
}

func (m *scriptModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    msg := m.steps[m.n]
    m.n++
    return msg, nil
}
func (m *scriptModel) Stream(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
    msg, err := m.Generate(ctx, in, opts...)
    if err != nil {
        return nil, err
    }
    return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}
func (m *scriptModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) { return m, nil }

type echoIn struct {
    Text string `json:"text" jsonschema:"required" jsonschema_description:"文本"`
}

func main() {
    ctx := context.Background()
    echo, err := utils.InferTool("echo", "回显", func(_ context.Context, in echoIn) (string, error) {
        return in.Text, nil
    })
    if err != nil {
        panic(err)
    }
    ag, err := react.NewAgent(ctx, &react.AgentConfig{
        ToolCallingModel: &scriptModel{steps: []*schema.Message{
            schema.AssistantMessage("", []schema.ToolCall{{
                ID: "1", Type: "function",
                Function: schema.FunctionCall{Name: "echo", Arguments: `{"text":"hi"}`},
            }}),
            schema.AssistantMessage("final", nil),
        }},
        ToolsConfig:     compose.ToolsNodeConfig{Tools: []tool.BaseTool{echo}},
        MaxStep:         8,
        MessageModifier: react.NewPersonaModifier("你是只读助手"),
    })
    if err != nil {
        panic(err)
    }
    out, err := ag.Generate(ctx, []*schema.Message{schema.UserMessage("echo hi")})
    if err != nil || out.Content != "final" {
        panic(out)
    }

    ma, err := host.NewMultiAgent(ctx, &host.MultiAgentConfig{
        Name: "demo",
        Host: host.Host{
            ToolCallingModel: &scriptModel{steps: []*schema.Message{
                schema.AssistantMessage("", []schema.ToolCall{{
                    ID: "h1", Type: "function",
                    Function: schema.FunctionCall{Name: "math", Arguments: `{"q":"1+1"}`},
                }}),
            }},
            SystemPrompt: "把数学题交给 math",
        },
        Specialists: []*host.Specialist{{
            AgentMeta: host.AgentMeta{Name: "math", IntendedUse: "整数运算"},
            Invokable: func(context.Context, []*schema.Message, ...agent.AgentOption) (*schema.Message, error) {
                return schema.AssistantMessage("2", nil), nil
            },
        }},
    })
    if err != nil {
        panic(err)
    }
    out, err = ma.Generate(ctx, []*schema.Message{schema.UserMessage("1+1")})
    if err != nil || out.Content != "2" {
        panic(out)
    }
}
```

`scriptModel.WithTools` 返回自身，这样 `NewAgent` 内部拿到的仍是同一个计数器。若 `WithTools` 值拷贝结构体，拷贝上的 `n` 从 0 开始，预设回复会被重复消费或提前用完。

## 9. ADK

### 9.1 类型

```go
type Message = *schema.Message
type AgentEvent = TypedAgentEvent[*schema.Message]

type TypedAgentEvent[M MessageType] struct {
    AgentName string
    RunPath   []RunStep
    Output    *TypedAgentOutput[M]
    Action    *AgentAction
    Err       error
}

type AgentOutput = TypedAgentOutput[*schema.Message]
type TypedAgentOutput[M MessageType] struct {
    MessageOutput   *TypedMessageVariant[M]
    CustomizedOutput any
}

type MessageVariant = TypedMessageVariant[*schema.Message]
type TypedMessageVariant[M MessageType] struct {
    IsStreaming   bool
    Message       M
    MessageStream *schema.StreamReader[M]
    Role          schema.RoleType
    ToolName      string
}

func (mv *TypedMessageVariant[M]) GetMessage() (M, error)

type AgentAction struct {
    Exit            bool
    Interrupted     *InterruptInfo
    TransferToAgent *TransferToAgentAction
    BreakLoop       *BreakLoopAction
    CustomizedAction any
}
```

`EnableStreaming == false` 时，`MessageOutput.Message` 是完整消息，`IsStreaming` 为 false。流式运行时 `IsStreaming` 为 true，正文在 `MessageStream` 里，只能由一个消费者 `Recv`。`GetMessage` 会把流收完并拼接。gocloudagent 自己读流，是为了在拼接前提取 `tool_call_delta`。

`Agent` 接口：

```go
type Agent interface {
    Name(ctx context.Context) string
    Description(ctx context.Context) string
    Run(ctx context.Context, input *AgentInput, options ...AgentRunOption) *AsyncIterator[*AgentEvent]
}
```

`ResumableAgent` 额外有 `Resume(ctx, *ResumeInfo, ...AgentRunOption)`。`ChatModelAgent` 实现了它。

```go
func (ai *AsyncIterator[T]) Next() (T, bool)
func NewAsyncIteratorPair[T any]() (*AsyncIterator[T], *AsyncGenerator[T])
```

`Next` 的 bool 为 false 表示迭代结束。事件上的 `Err` 是该步的失败，迭代不一定立刻结束，调用方要自己决定是否 break。gocloudagent 遇到 `event.Err != nil` 就停。

### 9.2 ChatModelAgent

```go
type ChatModelAgentConfig struct {
    Name                string
    Description         string
    Instruction         string
    Model               model.BaseModel[*schema.Message]
    ToolsConfig         ToolsConfig
    GenModelInput       GenModelInput
    Exit                tool.BaseTool
    OutputKey           string
    MaxIterations       int
    Middlewares         []AgentMiddleware // Deprecated
    Handlers            []ChatModelAgentMiddleware
    ModelRetryConfig    *ModelRetryConfig
    ModelFailoverConfig *ModelFailoverConfig[*schema.Message]
}

type ToolsConfig struct {
    compose.ToolsNodeConfig
    ReturnDirectly      map[string]bool
    EmitInternalEvents  bool
}

func NewChatModelAgent(ctx context.Context, config *ChatModelAgentConfig) (*ChatModelAgent, error)
```

`Instruction` 在默认 `GenModelInput` 里当作 f-string 模板，和 `adk` session 里的值替换，例如 `{User}`。指令正文里如果有字面量大括号（JSON 示例），默认格式化会失败。此时要提供自己的 `GenModelInput`，跳过 f-string。gocloudagent 的指令由 `AssembleInstruction` 拼好再传入，不依赖 session 占位符。

`MaxIterations` 默认 20，超过返回 `adk.ErrExceedMaxIterations`（`errors.New("exceeds max iterations")`）。gocloudagent 把这个错误收成正常结束：在正文后追加「已达到工具调用上限」，`RunResult.Reason = "max_iterations"`。

`Name` 和 `Description` 为空时，Agent 仍能单独跑，但不能作为 `NewAgentTool` 的子 Agent。子 Agent 通过：

```go
func SetSubAgents(ctx context.Context, agent Agent, subAgents []Agent) (ResumableAgent, error)
```

父 Agent 会自动带上 `transfer_to_agent` 工具。参数 JSON 是 `{"agent_name":"..."}`（`transferToAgent.InvokableRun`）。常量 `TransferToAgentToolName`。子 Agent 想把任务交回父 Agent 时，父级若调用过 `OnDisallowTransferToParent`，则不会提供这条回路。`WithDisallowTransferToParent` 是 `AgentOption`。

源码明确写了 `Exit` 和 `OutputKey` 不推荐用于「全家桶上下文共享」。子能力更稳妥的接法是 `adk.NewAgentTool(ctx, agent, ...AgentToolOption)`，父模型把它当普通工具调用，子 Agent 的可见历史由 `WithFullChatHistoryAsInput` 或 `WithAgentInputSchema` 控制。

`Handlers` 是 `ChatModelAgentMiddleware`。嵌入 `BaseChatModelAgentMiddleware` 后只覆盖关心的方法：

- `BeforeAgent`：改 `ChatModelAgentContext.Tools`，影响本轮真正可执行的工具和送给模型的 schema。
- `BeforeModelRewriteState` / `AfterModelRewriteState`：改 `ChatModelAgentState.Messages` 和 `ToolInfos`。改 `ToolInfos` 会留在 state 里，后续轮次都生效。
- `WrapModel`：包一层模型。在这里用 `model.WithTools` 临时换工具不会写回 state，也会打乱 prompt cache。
- `WrapInvokableToolCall` 等：包单次工具执行。

`AgentMiddleware` 结构体（`AdditionalInstruction`、`AdditionalTools`、`BeforeChatModel`、`AfterChatModel`、`WrapToolCall`）已标记废弃，处理顺序仍在 `ChatModelAgentConfig` 的注释里：结构体 middleware 先于 interface handler。

执行顺序（模型调用，外到内）摘要：`BeforeChatModel` → `BeforeModelRewriteState` → failover → retry → 默认事件发送 → 用户 `WrapModel` → 真正的 `Generate`/`Stream` → `AfterModelRewriteState` → `AfterChatModel`。工具调用默认先发事件，再走 `ToolsConfig.ToolCallMiddlewares`、`WrapToolCall`、handler。若自己把 `NewEventSenderModelWrapper` 或 `NewEventSenderToolWrapper` 放进 `Handlers`，框架不再插入默认发送器。

`ModelRetryConfig` 见第 13 节。`ModelFailoverConfig` 要求同时提供 `GetFailoverModel` 和 `ShouldFailover`，否则 `NewChatModelAgent` 直接返回错误。

### 9.3 Runner

```go
type RunnerConfig struct {
    Agent            Agent
    EnableStreaming  bool
    CheckPointStore  CheckPointStore
}

func NewRunner(ctx context.Context, conf RunnerConfig) *Runner
func (r *Runner) Run(ctx context.Context, messages []Message, opts ...AgentRunOption) *AsyncIterator[*AgentEvent]
func (r *Runner) Query(ctx context.Context, query string, opts ...AgentRunOption) *AsyncIterator[*AgentEvent]
func (r *Runner) Resume(ctx context.Context, checkPointID string, opts ...AgentRunOption) (*AsyncIterator[*AgentEvent], error)
func (r *Runner) ResumeWithParams(ctx context.Context, checkPointID string, params *ResumeParams, opts ...AgentRunOption) (*AsyncIterator[*AgentEvent], error)

type ResumeParams struct {
    Targets map[string]any
}
```

`Query` 等价于一条 `UserMessage`。`EnableStreaming` 决定 `AgentInput.EnableStreaming`，进而决定模型走 `Stream` 还是 `Generate`。gocloudagent 把它设成请求上的 `req.Stream`。

`CheckPointStore`：

```go
type CheckPointStore interface {
    Get(ctx context.Context, checkPointID string) ([]byte, bool, error)
    Set(ctx context.Context, checkPointID string, checkPoint []byte) error
}
```

可选 `CheckPointDeleter`：`Delete(ctx, id) error`。没实现的话框架不会自动删旧快照。

运行 option：

```go
func WithCheckPointID(id string) AgentRunOption
func WithChatModelOptions(opts []model.Option) AgentRunOption
func WithToolOptions(opts []tool.Option) AgentRunOption
func WithCallbacks(handlers ...callbacks.Handler) AgentRunOption
func WithSessionValues(v map[string]any) AgentRunOption
func WithSkipTransferMessages() AgentRunOption
```

Session：

```go
func AddSessionValue(ctx context.Context, key string, value any)
func AddSessionValues(ctx context.Context, kvs map[string]any)
func GetSessionValue(ctx context.Context, key string) (any, bool)
func GetSessionValues(ctx context.Context) map[string]any
```

`OutputKey` 会把助手正文 `AddSessionValue`。gocloudagent 不用 session，项目、用户、run id 放在自己的 `context` value（`withRunContext`）里，工具从那里取，避免把凭据写进模型可见的 session。

### 9.4 子 Agent、工作流、预置模式

```go
func NewSequentialAgent(ctx context.Context, config *SequentialAgentConfig) (ResumableAgent, error)
func NewParallelAgent(ctx context.Context, config *ParallelAgentConfig) (ResumableAgent, error)
func NewLoopAgent(ctx context.Context, config *LoopAgentConfig) (ResumableAgent, error)
func NewBreakLoopAction(agentName string) *AgentAction
```

配置都是 `Name`、`Description`、`SubAgents`。`LoopAgentConfig` 还有 `MaxIterations`。循环里的子 Agent 发出 `BreakLoop` 且 `Done == true` 时结束。

预置包：

| 函数 | 说明 |
| --- | --- |
| `supervisor.New(ctx, *supervisor.Config) (adk.ResumableAgent, error)` | `Config{Supervisor, SubAgents}`。子 Agent 被包成只能交回 supervisor。源码标记为不推荐，优先 AgentTool 或 Deep |
| `planexecute.New(ctx, *planexecute.Config)` | 计划、执行、重规划。另有 `NewPlanner`、`NewExecutor`、`NewReplanner` |
| `deep.New(ctx, *deep.Config)` | 带文件系统、shell、todo、子 Agent 的 Deep agent。`MaxIteration`、`Backend`、`Shell` 都在 `Config` 里 |

gocloudagent 的「多能力」不是多 Agent：一个 `ChatModelAgent`，工具列表由模块和 skill 决定。沙盒四个工具和 CLI 工具挂在同一个 `ToolsNodeConfig.Tools` 上。

### 9.5 可运行示例

```go
package main

import (
    "context"
    "strings"
    "sync"

    "github.com/cloudwego/eino/adk"
    "github.com/cloudwego/eino/components/model"
    "github.com/cloudwego/eino/components/tool"
    "github.com/cloudwego/eino/components/tool/utils"
    "github.com/cloudwego/eino/compose"
    "github.com/cloudwego/eino/schema"
)

type scriptModel struct {
    mu sync.Mutex
    steps []*schema.Message
    n int
}

func (m *scriptModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    msg := m.steps[m.n]
    m.n++
    return msg, nil
}
func (m *scriptModel) Stream(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
    msg, err := m.Generate(ctx, in, opts...)
    if err != nil {
        return nil, err
    }
    return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}
func (m *scriptModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) { return m, nil }

type echoIn struct {
    Text string `json:"text" jsonschema:"required" jsonschema_description:"文本"`
}

func drain(iter *adk.AsyncIterator[*adk.AgentEvent]) []*adk.AgentEvent {
    var out []*adk.AgentEvent
    for {
        ev, ok := iter.Next()
        if !ok {
            return out
        }
        out = append(out, ev)
    }
}

func main() {
    ctx := context.Background()
    echo, err := utils.InferTool("echo", "回显", func(_ context.Context, in echoIn) (string, error) {
        return in.Text, nil
    })
    if err != nil {
        panic(err)
    }
    agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
        Name:        "reader",
        Description: "回答问题",
        Instruction: "需要时调用 echo",
        Model: &scriptModel{steps: []*schema.Message{
            schema.AssistantMessage("", []schema.ToolCall{{
                ID: "1", Type: "function",
                Function: schema.FunctionCall{Name: "echo", Arguments: `{"text":"yo"}`},
            }}),
            schema.AssistantMessage("answered", nil),
        }},
        ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{
            Tools: []tool.BaseTool{echo},
        }},
        MaxIterations: 4,
    })
    if err != nil {
        panic(err)
    }
    runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: agent, EnableStreaming: false})
    var text string
    for _, ev := range drain(runner.Run(ctx, []*schema.Message{schema.UserMessage("yo")})) {
        if ev.Err != nil {
            panic(ev.Err)
        }
        if ev.Output == nil || ev.Output.MessageOutput == nil || ev.Output.MessageOutput.Message == nil {
            continue
        }
        if ev.Output.MessageOutput.Message.Role == schema.Assistant {
            text += ev.Output.MessageOutput.Message.Content
        }
    }
    if !strings.Contains(text, "answered") {
        panic(text)
    }

    step1, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
        Name: "s1", Description: "first", Instruction: "say one",
        Model: &scriptModel{steps: []*schema.Message{schema.AssistantMessage("one", nil)}},
    })
    if err != nil {
        panic(err)
    }
    step2, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
        Name: "s2", Description: "second", Instruction: "say two",
        Model: &scriptModel{steps: []*schema.Message{schema.AssistantMessage("two", nil)}},
    })
    if err != nil {
        panic(err)
    }
    seq, err := adk.NewSequentialAgent(ctx, &adk.SequentialAgentConfig{
        Name: "seq", Description: "two steps", SubAgents: []adk.Agent{step1, step2},
    })
    if err != nil {
        panic(err)
    }
    seqRunner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: seq, EnableStreaming: false})
    var parts []string
    for _, ev := range drain(seqRunner.Query(ctx, "go")) {
        if ev.Err != nil {
            panic(ev.Err)
        }
        if ev.Output != nil && ev.Output.MessageOutput != nil && ev.Output.MessageOutput.Message != nil &&
            ev.Output.MessageOutput.Message.Content != "" {
            parts = append(parts, ev.AgentName+":"+ev.Output.MessageOutput.Message.Content)
        }
    }
    if len(parts) < 2 {
        panic(strings.Join(parts, ","))
    }
}
```

## 10. Callbacks

```go
type RunInfo struct {
    Name      string
    Type      string
    Component components.Component
}

type Handler interface {
    OnStart(ctx context.Context, info *RunInfo, input CallbackInput) context.Context
    OnEnd(ctx context.Context, info *RunInfo, output CallbackOutput) context.Context
    OnError(ctx context.Context, info *RunInfo, err error) context.Context
    OnStartWithStreamInput(ctx context.Context, info *RunInfo, input *schema.StreamReader[CallbackInput]) context.Context
    OnEndWithStreamOutput(ctx context.Context, info *RunInfo, output *schema.StreamReader[CallbackOutput]) context.Context
}
```

`RunInfo` 可能是 nil：组件直接调 `OnStart` 却没先 `EnsureRunInfo` 时就是这样。读字段前要判断。`Component` 取值见 `components.ComponentOfChatModel`（`"ChatModel"`）、`ComponentOfTool`、`ComponentOfPrompt`、`ComponentOfEmbedding` 等。图节点另有 `compose.ComponentOfLambda`、`ComponentOfToolsNode`、`ComponentOfGraph`。

时机常量：`TimingOnStart`、`TimingOnEnd`、`TimingOnError`、`TimingOnStartWithStreamInput`、`TimingOnEndWithStreamOutput`。实现 `TimingChecker` 可以让框架跳过不关心的时机，避免为了回调去复制整条流。

```go
func NewHandlerBuilder() *HandlerBuilder
func (hb *HandlerBuilder) OnStartFn(...) *HandlerBuilder
func (hb *HandlerBuilder) OnEndFn(...) *HandlerBuilder
func (hb *HandlerBuilder) OnErrorFn(...) *HandlerBuilder
func (hb *HandlerBuilder) OnStartWithStreamInputFn(...) *HandlerBuilder
func (hb *HandlerBuilder) OnEndWithStreamOutputFn(...) *HandlerBuilder
func (hb *HandlerBuilder) Build() Handler

func InitCallbacks(ctx context.Context, info *RunInfo, handlers ...Handler) context.Context
func AppendGlobalHandlers(handlers ...Handler)
func OnStart[T any](ctx context.Context, input T) context.Context
func OnEnd[T any](ctx context.Context, output T) context.Context
func OnError(ctx context.Context, err error) context.Context
func OnEndWithStreamOutput[T any](ctx context.Context, output *schema.StreamReader[T]) (context.Context, *schema.StreamReader[T])
```

全局 handler 对进程内所有后续运行生效，测试里容易串。单次调用用 `compose.WithCallbacks` 或 `adk.WithCallbacks`。

`utils/callbacks`（import 路径 `github.com/cloudwego/eino/utils/callbacks`）按组件拆 handler：

```go
helper := template.NewHandlerHelper().
    ChatModel(&template.ModelCallbackHandler{
        OnEnd: func(ctx context.Context, info *callbacks.RunInfo, output *model.CallbackOutput) context.Context {
            return ctx
        },
    }).
    Tool(&template.ToolCallbackHandler{
        OnStart: func(ctx context.Context, info *callbacks.RunInfo, input *tool.CallbackInput) context.Context {
            return ctx
        },
    }).
    Handler()
```

`ModelCallbackHandler.OnEnd` 拿到的是已经转好的 `*model.CallbackOutput`，里面有 `Message` 和 `TokenUsage`。流式结束走 `OnEndWithStreamOutput`，参数是 `*schema.StreamReader[*model.CallbackOutput]`。ADK Agent 另有 `template` 的 agent handler，`ConvAgentCallbackInput` 能转出 `*adk.AgentCallbackInput`。

组件作者要自己发回调时：`callbacks.EnsureRunInfo` 放进 ctx，`IsCallbacksEnabled` 返回 true，在开始和结束调用 `OnStart` / `OnEnd`。流式输出使用 `OnEndWithStreamOutput`，它会复制流，一份给回调、一份返回给调用方。

```go
package main

import (
    "context"
    "sync"

    "github.com/cloudwego/eino/callbacks"
    "github.com/cloudwego/eino/components"
    "github.com/cloudwego/eino/components/model"
    "github.com/cloudwego/eino/components/prompt"
    "github.com/cloudwego/eino/compose"
    template "github.com/cloudwego/eino/utils/callbacks"
    "github.com/cloudwego/eino/schema"
)

type scriptModel struct {
    mu sync.Mutex
    steps []*schema.Message
    n int
}

func (m *scriptModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    msg := m.steps[m.n]
    m.n++
    return msg, nil
}
func (m *scriptModel) Stream(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
    msg, err := m.Generate(ctx, in, opts...)
    if err != nil {
        return nil, err
    }
    return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}
func (m *scriptModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) { return m, nil }

func main() {
    ctx := context.Background()
    var starts int
    handler := callbacks.NewHandlerBuilder().OnStartFn(
        func(ctx context.Context, info *callbacks.RunInfo, _ callbacks.CallbackInput) context.Context {
            if info != nil && info.Component == components.ComponentOfChatModel {
                starts++
            }
            return ctx
        },
    ).Build()

    chain := compose.NewChain[map[string]any, *schema.Message]()
    chain.AppendChatTemplate(prompt.FromMessages(schema.FString, schema.UserMessage("{q}"))).
        AppendChatModel(&scriptModel{steps: []*schema.Message{schema.AssistantMessage("cb", nil)}})
    r, err := chain.Compile(ctx)
    if err != nil {
        panic(err)
    }
    if _, err = r.Invoke(ctx, map[string]any{"q": "hi"}, compose.WithCallbacks(handler)); err != nil || starts < 1 {
        panic(err)
    }

    helper := template.NewHandlerHelper().ChatModel(&template.ModelCallbackHandler{
        OnEnd: func(ctx context.Context, _ *callbacks.RunInfo, output *model.CallbackOutput) context.Context {
            if output == nil || output.Message == nil || output.Message.Content != "cb" {
                panic("callback output")
            }
            return ctx
        },
    }).Handler()
    chain2 := compose.NewChain[map[string]any, *schema.Message]()
    chain2.AppendChatTemplate(prompt.FromMessages(schema.FString, schema.UserMessage("{q}"))).
        AppendChatModel(&scriptModel{steps: []*schema.Message{schema.AssistantMessage("cb", nil)}})
    r2, err := chain2.Compile(ctx)
    if err != nil {
        panic(err)
    }
    if _, err = r2.Invoke(ctx, map[string]any{"q": "hi"}, compose.WithCallbacks(helper)); err != nil {
        panic(err)
    }
}
```

回调里不要打印 `Message.Content` 或工具参数到生产日志。本仓库审计只记录哈希和元数据。

## 11. 中断、恢复与 Checkpoint

有两层地址。compose 的段类型是 `node`、`tool`、`runnable`。ADK 再加 `agent`。一次工具里的 `tool.StatefulInterrupt` 会变成 ADK 事件的 `Action.Interrupted`，里面有一组 `*InterruptCtx`。`IsRootCause` 标出真正发起中断的那一层，恢复数据要填到这个 id 上。

### 11.1 图级中断

编译时声明在哪些节点之前或之后停住，并提供 store：

```go
r, err := g.Compile(ctx,
    compose.WithCheckPointStore(store),
    compose.WithInterruptAfterNodes([]string{"1"}),
    compose.WithInterruptBeforeNodes([]string{"2"}),
    compose.WithGraphName("root"),
)
_, err = r.Invoke(ctx, "start", compose.WithCheckPointID("1"))
info, ok := compose.ExtractInterruptInfo(err)
```

`ExtractInterruptInfo` 的 `InterruptInfo` 含 `State`、`BeforeNodes`、`AfterNodes`、`RerunNodes`、`SubGraphs`、`InterruptContexts`。

恢复：

```go
func Resume(ctx context.Context, interruptIDs ...string) context.Context
func ResumeWithData(ctx context.Context, interruptID string, data any) context.Context
func BatchResumeWithData(ctx context.Context, resumeData map[string]any) context.Context
```

用同一个 `CheckPointID` 再 `Invoke`。`WithForceNewRun` 忽略已有快照。`WithStateModifier` 在恢复前改 state。`WithWriteToCheckPointID` 把本次快照写到另一个 id。

节点内部也可以主动中断：

```go
func Interrupt(ctx context.Context, info any) error
func StatefulInterrupt(ctx context.Context, info any, state any) error
func GetInterruptState[T any](ctx context.Context) (wasInterrupted bool, hasState bool, state T)
func GetResumeContext[T any](ctx context.Context) (isResumeFlow bool, hasData bool, data T)
```

`IsInterruptRerunError` 用来识别「中断并重跑当前节点」。

状态类型在 `init` 里注册：

```go
func Register[T any]()
func RegisterName[T any](name string)
```

`Register` 会 panic 当注册失败。同一个进程对同一类型注册两次也会 panic，所以放在 `init`，测试不要再注册一遍。

### 11.2 工具级 human-in-the-loop

工具在 ADK 或带 checkpoint 的 ToolsNode 里暂停时，模式是：

1. 第一次进入：`wasInterrupted == false`，返回 `tool.StatefulInterrupt(ctx, info, state)`。`info` 给 UI 看，`state` 留给下次执行。
2. 恢复时如果这个中断点不是目标，原样再 `StatefulInterrupt` 一次，把 `state` 交回去。
3. `GetResumeContext` 表明自己是目标且带了数据，才真正执行。

`info` 和 `state` 的具体类型都要 `schema.Register`。ADK 恢复：

```go
iter, err := runner.ResumeWithParams(ctx, checkpointID, &adk.ResumeParams{
    Targets: map[string]any{
        interruptID: &approvalResult{Approved: true},
    },
})
```

`interruptID` 来自 `event.Action.Interrupted.InterruptContexts` 里 `IsRootCause == true` 的 `ID`。`Runner` 必须带着实现了 `CheckPointStore` 的配置创建，并且第一次 `Run` 传入 `adk.WithCheckPointID`。

`Resume` 不带 `Targets`，适合不需要把数据交回工具的中断。`WithHistoryModifier` 已废弃，恢复时改历史用 `ChatModelAgentResumeData`。

图中断和工具中断可以嵌套。`compose.CompositeInterrupt` / `tool.CompositeInterrupt` 把子中断收成一个错误。v0.9.21 补上了「流式工具收到取消信号时，checkpoint 仍能记下已完成的兄弟工具并只重跑被取消的那个」；v0.9.19 没有这段逻辑。

gocloudagent 的人工确认**没有**用 `tool.StatefulInterrupt`。破坏性 CLI 在 `execSimplebase` 里发 `confirmation_required` 事件，通过 `ConfirmationWaiter.Wait` 阻塞，超时或拒绝时返回一段错误 JSON 给模型。定时任务（`Headless`）直接拒绝。这是产品层的确认，不是 Eino checkpoint。因此云 Agent 进程重启后不能从半次工具调用恢复。

### 11.3 可运行示例

内存 store 与下面两个 `main` 都在 v0.9.19 上跑通。第一个是图在节点之间暂停；第二个是 ADK 工具暂停后 `ResumeWithParams`。

```go
package main

import (
    "context"
    "sync"

    "github.com/cloudwego/eino/compose"
    "github.com/cloudwego/eino/schema"
)

type memStore struct {
    mu sync.Mutex
    m  map[string][]byte
}

func (s *memStore) Get(_ context.Context, id string) ([]byte, bool, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    b, ok := s.m[id]
    if !ok {
        return nil, false, nil
    }
    return append([]byte(nil), b...), true, nil
}

func (s *memStore) Set(_ context.Context, id string, data []byte) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    if s.m == nil {
        s.m = map[string][]byte{}
    }
    s.m[id] = append([]byte(nil), data...)
    return nil
}

type cpState struct{ A string }

func init() { schema.Register[*cpState]() }

func main() {
    ctx := context.Background()
    g := compose.NewGraph[string, string](compose.WithGenLocalState(func(context.Context) *cpState {
        return &cpState{}
    }))
    _ = g.AddLambdaNode("1", compose.InvokableLambda(func(_ context.Context, in string) (string, error) {
        return in + "1", nil
    }))
    _ = g.AddLambdaNode("2", compose.InvokableLambda(func(_ context.Context, in string) (string, error) {
        return in + "2", nil
    }), compose.WithStatePreHandler(func(_ context.Context, in string, state *cpState) (string, error) {
        return in + state.A, nil
    }))
    _ = g.AddEdge(compose.START, "1")
    _ = g.AddEdge("1", "2")
    _ = g.AddEdge("2", compose.END)
    r, err := g.Compile(ctx,
        compose.WithCheckPointStore(&memStore{}),
        compose.WithInterruptAfterNodes([]string{"1"}),
        compose.WithInterruptBeforeNodes([]string{"2"}),
        compose.WithGraphName("root"),
    )
    if err != nil {
        panic(err)
    }
    _, err = r.Invoke(ctx, "start", compose.WithCheckPointID("1"))
    info, ok := compose.ExtractInterruptInfo(err)
    if !ok || len(info.InterruptContexts) == 0 {
        panic(err)
    }
    resumed := compose.ResumeWithData(ctx, info.InterruptContexts[0].ID, &cpState{A: "state"})
    result, err := r.Invoke(resumed, "start", compose.WithCheckPointID("1"))
    if err != nil || result != "start1state2" {
        panic(result)
    }
}
```

工具中断示例见仓库外的验证程序，结构与 `adk/prebuilt/integration_test.go` 的 `approvableTool` 相同，核心如下（类型需要在 `init` 里 `schema.Register`）：

```go
func (askTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
    wasInterrupted, _, stored := tool.GetInterruptState[string](ctx)
    if !wasInterrupted {
        return "", tool.StatefulInterrupt(ctx, &approvalInfo{
            ToolName: "ask", ArgumentsInJSON: argumentsInJSON,
            ToolCallID: compose.GetToolCallID(ctx),
        }, argumentsInJSON)
    }
    isTarget, hasData, data := tool.GetResumeContext[*approvalResult](ctx)
    if !isTarget || !hasData {
        return "", tool.StatefulInterrupt(ctx, &approvalInfo{
            ToolName: "ask", ArgumentsInJSON: stored,
            ToolCallID: compose.GetToolCallID(ctx),
        }, stored)
    }
    if !data.Approved {
        return "denied", nil
    }
    return "approved:" + stored, nil
}
```

配合 `EnableStreaming: false` 的 Runner、`adk.WithCheckPointID("hitl")`，从事件里取出 root interrupt id 后：

```go
iter, err := runner.ResumeWithParams(ctx, "hitl", &adk.ResumeParams{
    Targets: map[string]any{interruptID: &approvalResult{Approved: true}},
})
```

恢复后的工具消息正文包含 `approved:`，随后脚本模型返回最终 assistant 文本。

## 12. 流式输出与 tool call 增量

供应商的流通常是这个形状：第一块带 `id` 和 `name`，后续块只有 `arguments` 的几十字节，`index` 指出它属于第几个并行工具调用。Eino 的合并函数 `concatToolCalls`（`schema/message.go`）按 `*Index` 分组：

- 同一 index：`ID`、`Type`、`Function.Name` 取第一个非空值，若后续块给出不同的非空值则报错；`Arguments` 无条件拼接。
- `Index == nil`：该 chunk 单独保留，不和任何人拼接。

因此适配器必须：

1. 每个 delta 都带上同一个 `*int` index。
2. `Function.Name` 只在第一次非空，后续块留空。空字符串会被 concat 忽略，不会当成名称冲突。
3. `Arguments` 只放本块新增的片段。每块都放累计全文时，拼接结果会重复。
4. 同一 index 上的非空 `ID` 必须始终相同。第一块拿到真实 id 之后，后续块复用它。在 id 到达前写占位符、之后再改成真实 id，会触发 `cannot concat ToolCalls with different tool id`。
5. 文本 delta 和工具 delta 可以出现在同一条流里。`ConcatMessages` 会同时拼接 `Content` 和 `ToolCalls`。若业务上要忽略工具阶段的正文，在适配器里丢掉。

gocloudagent 的 `gatewayChatModel.Stream`（`model.go`）对上游 `DeltaStream` 做了这件事：按 `tc.Index` 在本地累加，同时向 Eino 发送带 `Index` 和 `Extra["delta"]=true` 的 chunk。注释写明：不带 index 时每个片段都是独立 ToolCall，空 name 会让工具节点报 not found。

文本协议（`protocol == "text"`，也是 `newGatewayChatModel` 的默认值）不走原生 tool call。模型被要求在正文里输出 `TOOL_CALL` 前缀。`Stream` 会把开头若干字符先攒在 `pending` 里，直到能判断它不是这个前缀，再一次性放出，避免把标记拆进 token 事件。流结束时若仍停在工具模式，则 `parseAssistant` 解析整段。

ADK 开启 `EnableStreaming` 后，助手事件的 `MessageStream` 由 gocloudagent 的 `consumeAssistant` 独占读取：

- chunk 的第一个 `ToolCall` 若 `Extra["delta"]==true`，或 id 以 `delta:` 开头（`isToolDelta`），就发 SSE 事件 `tool_call_delta`，并用 `mergeStreamToolCall` 按 index 在本地拼好。
- 纯文本 chunk 追加到本轮 assistant buffer，并发 `token` 事件。
- 流结束后如果本地 merged 非空，返回一条只含完整 `ToolCalls` 的 assistant 消息。`StartRun` 再为每个 call 发 `tool_call` 事件。

`mergeStreamToolCall` 与 `ConcatMessages` 的差异：它按 index 找已有元素，name 只在空时填入，arguments 直接 `+=`，并且会去掉 id 前缀 `delta:`。它不检查「同一 index 上 name 冲突」。适配器如果违规地在后续块改了 name，这里会保留第一块的 name，而 `ConcatMessages` 会返回错误。两条路径要同时满足，增量 chunk 就保持 name 只出现一次。

`consumeVariant` 读工具结果流，把 `Content` 拼起来，`ToolName` 取最后一个非空值。`MessageStream` 读到任何非 EOF 错误都会停，已读到的正文仍然返回。调用方若还要把这个错误当成失败，需要另存 err；当前实现把非 EOF 和 EOF 同样 break。

`schema.StreamReader.Copy` 必须发生在任何 `Recv` 之前。ReAct 的工具结果收集 middleware 对 `Streamable` 结果 `Copy(2)`，一份送给旁路，一份留给节点。已经读过的 reader 再 `Copy` 会得到不完整的后半段。

`Pipe` 的容量是缓冲块数，不是字节。gocloudagent 使用 `schema.Pipe[*schema.Message](8)`。发送 goroutine在 `defer sw.Close()` 之后还要 `defer stream.Close()` 关掉上游。发送失败用 `sw.Send(nil, err)`，接收方 `Recv` 得到该错误，零值消息可以是 nil，接收方要跳过 nil chunk（`consumeAssistant` 就是这样做的）。

## 13. 错误、超时与重试

### 13.1 上下文

把 `context.WithTimeout` 传给 `Invoke`、`Runner.Run` 或工具函数。节点应监听 `ctx.Done()`。实测：图中的 lambda `select` 到 `ctx.Done()` 后，`Invoke` 返回的错误满足 `errors.Is(err, context.DeadlineExceeded)`。

gocloudagent 在 `StartRun` 里用 `r.RunTimeout`，默认 180 秒，包住整轮 Agent，而不是单次模型调用。取消函数按 `RunID` 存在 `Runtime.cancels`，供外部取消。`event.Err` 或 `runCtx.Err()` 为 `context.Canceled` 时，`RunResult.Reason = "canceled"` 并返回 `context.Canceled`。超时返回 `context.DeadlineExceeded`。

工具里的 `context.Canceled` / `DeadlineExceeded` 不要包成普通工具错误，否则模型会以为这是业务失败并继续规划。`errorTolerantTool` 把这两类原样抛出。

### 13.2 ADK 重试

```go
type ModelRetryConfig struct {
    MaxRetries   int
    ShouldRetry  func(ctx context.Context, retryCtx *RetryContext) *RetryDecision
    IsRetryAble  func(ctx context.Context, err error) bool
    BackoffFunc  func(ctx context.Context, attempt int) time.Duration
}
```

`MaxRetries` 是失败后的额外次数。`attempt` 从 0 到 `MaxRetries` 含首尾，所以 `MaxRetries: 1` 最多调用模型两次。

未设置 `ShouldRetry` 时走 legacy：`IsRetryAble == nil` 则 `defaultIsRetryAble` 对任意非 nil 错误返回 true。中断错误和 `ErrStreamCanceled` 不重试。默认退避从 100ms 指数增长到 10s，带抖动。测试里把 `BackoffFunc` 设为返回 0。

重试耗尽返回 `*adk.RetryExhaustedError`，`Unwrap` 是最后一次错误，另有 `ErrExceedMaxRetries`。

`ShouldRetry` 路径可以改写输入消息（`RetryDecision.ModifiedInputMessages`）或把错误换成 `RewriteError`。流式重试比同步重试更严：已经向下游发出的 chunk 不能无声地重来。适配器应在「首个有效 chunk 之前」失败，让重试还有意义。

`ModelFailoverConfig` 在重试之外切换模型实例。`GetFailoverModel` 与 `ShouldFailover` 都必填。

### 13.3 工具错误与未知工具

`ToolsNode` 里某个工具返回 error，默认整次 `Invoke` 失败。若希望模型看到错误并换一条路：

- `utils.WrapInvokableToolWithErrorHandler` 把 error 换成字符串，`InvokableRun` 返回 nil error。
- `UnknownToolsHandler` 专门接模型幻觉出来的工具名。
- gocloudagent 的 `wrapToolErrors` 对每个 `InvokableTool` 包一层，业务错误变成 JSON `{"error":"...","is_error":true}`。

`SetReturnDirectly`（react 包）让当前轮次在工具执行后直接结束，适合「工具输出就是最终答案」的场景。

### 13.4 步数耗尽

| 位置 | 配置 | 默认 | 错误 |
| --- | --- | --- | --- |
| compose 图 | `WithMaxRunSteps` | 与图的 pregel 实现有关；react 注释写节点数 + 10 | 运行步数错误 |
| `react.AgentConfig.MaxStep` | 同上 | 12 | 步数耗尽 |
| `adk.ChatModelAgentConfig.MaxIterations` | 模型生成轮数 | 20 | `adk.ErrExceedMaxIterations` |
| gocloudagent `Runtime.MaxIterations` | 传入 ADK | 配置为 0 时用 8 | 捕获后写入 `Reason=max_iterations`，函数返回 nil error |

## 14. 测试

Eino 的 handler 测试不需要真实供应商。一个脚本模型只要实现被测路径会调用的方法。

`react.Agent` 和 host 需要 `ToolCallingChatModel`：`Generate`、`Stream`、`WithTools`。ADK 的 `ChatModelAgent` 只要求 `BaseChatModel`（`Generate` + `Stream`），工具通过 `model.WithTools` option 传入。为了同一假模型两边都能用，把 `WithTools` 也实现上，并**返回同一个指针**，让预设回复的游标在框架内部的 `WithTools` 之后仍然有效。

```go
type scriptModel struct {
    mu    sync.Mutex
    steps []*schema.Message
    n     int
    tools []*schema.ToolInfo
}

func (m *scriptModel) Generate(_ context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
    if opt := model.GetCommonOptions(&model.Options{}, opts...); len(opt.Tools) > 0 {
        m.tools = opt.Tools
    }
    m.mu.Lock()
    defer m.mu.Unlock()
    if m.n >= len(m.steps) {
        return schema.AssistantMessage("done", nil), nil
    }
    msg := m.steps[m.n]
    m.n++
    return msg, nil
}

func (m *scriptModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
    msg, err := m.Generate(ctx, input, opts...)
    if err != nil {
        return nil, err
    }
    // 要测增量拼接时，在这里改成按 Index 发送多个 chunk，而不是整段一条。
    return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func (m *scriptModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
    m.mu.Lock()
    m.tools = tools
    m.mu.Unlock()
    return m, nil
}
```

流式断言直接读 `StreamReader`，把 chunk 交给 `schema.ConcatMessages`，再检查拼好的 `ToolCalls[i].Function.Arguments` 是合法 JSON。缺 `Index` 的回归测试应该断言 `len(ToolCalls) > 1`。

工具用 `InferTool` 配一个纯函数即可，参数用最小结构体。检查 `ToolsNode.Invoke` 的返回消息 `Role`、`ToolCallID`、`Content`。

Checkpoint 测试用内存 `CheckPointStore`（第 11 节的 `memStore`）。`Get` 要返回副本，避免调用方改到 store 内部的切片。状态类型在 `init` 注册一次。

中断测试断言 `ExtractInterruptInfo` 或 `Action.Interrupted`，再用同一 checkpoint id 恢复，断言工具只在恢复后产生副作用。第一次调用就写数据库的工具，无法安全地做 human-in-the-loop。

超时测试把超时设得很短（几十毫秒），lambda 里 `select` `ctx.Done()`，断言 `errors.Is(..., context.DeadlineExceeded)`。

gocloudagent 自己的测试在 `runtime_test.go`、`model_native_test.go`、`optimization_test.go`、`coverage_test.go`。它们构造 `schema.Pipe`，喂给 `consumeAssistant`，覆盖 delta、普通 token 和流错误。模型测试把若干 chunk `schema.ConcatMessages`，确认原生 tool call 能收成一次调用。新增流式行为时优先加在这些测试里，用假的 `ChatClient`，不要打外部 LLM。

本文示例的验证方式：临时模块 `go get github.com/cloudwego/eino@v0.9.19` 后 `go run`，覆盖 schema 拼接、InferTool/NewTool/ToolsNode、Chain/Graph/分支/并行/Workflow/State、四类 Runnable 语义、react、host、ADK Runner、顺序 Agent、callbacks、图 checkpoint、工具 `StatefulInterrupt`、`ModelRetryConfig`、超时。该目录不在本仓库内。

## 15. 在 SimpleBase gocloudagent 中如何使用

云 Agent 是一个 ADK `ChatModelAgent`。入口是 `Runtime.StartRun`（`runtime.go`）。它不编译 Chain/Graph，也不使用 `react.Agent`。

### 15.1 调用链

```text
StartRun
  claimThread
  AssembleInstruction          prompt.go
  buildTools 或 buildSkillTools
  newGatewayChatModel          model.go
  adk.NewChatModelAgent
  adk.NewRunner(EnableStreaming = req.Stream)
  historyToSchema + UserMessage
  runner.Run
  循环 iter.Next
      tool 角色 → consumeVariant → tool_result 事件
      assistant → consumeAssistant → token / tool_call_delta / tool_call
```

`adk.NewChatModelAgent` 的配置与源码对应关系：

| 字段 | 来源 |
| --- | --- |
| `Name` | `safeAgentName(req.Agent.Name)`，空白时为 `"agent"` |
| `Description` | `req.Agent.Description` |
| `Instruction` | `AssembleInstruction` |
| `Model` | `*gatewayChatModel` |
| `ToolsConfig.Tools` | `buildTools` 或 `buildSkillTools` 的结果 |
| `MaxIterations` | `Runtime.MaxIterations`，小于等于 0 时为 8 |

`RunnerConfig` 只有 `Agent` 和 `EnableStreaming`。没有 `CheckPointStore`。

### 15.2 模型适配 `gatewayChatModel`

文件：`model.go`。

```go
func (m *gatewayChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error)
func (m *gatewayChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error)
func (m *gatewayChatModel) BindTools(tools []*schema.ToolInfo) error
func (m *gatewayChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error)
```

`newGatewayChatModel` 把 `protocol` 设为 `"text"`。`Runtime.ToolProtocol` 非空时覆盖它。

- `text`：不使用供应商原生 tools 字段。`toolProtocolPrompt` 把 `ToolInfo` 写进一条额外 system 消息，模型用正文里的 `TOOL_CALL` 标记发起调用，`parseAssistant` 再转成 `schema.ToolCall`。
- `native`：`buildRequest` 把 `ParamsOneOf.ToJSONSchema()` 放进 `ChatRequest.Tools`，assistant 的 `ToolCalls` 和 tool 角色的 `ToolCallID` 按供应商消息传递。
- `auto`：先按 native 请求；若上游 `LiteLLMError` 状态码是 400 或 422 且错误文本里有 `tool` 或 `function`，把 `project|model` 记入 `unsupportedNativeTools`，同一进程内后续请求退回 text。

`Generate` 读取 `model.GetCommonOptions`。`options.Model` 非空时覆盖 Agent 的 `ModelOverride` 和项目默认模型。有 `resp.ToolCalls` 时走 `assistantToolMessage`，否则 `parseAssistant`。

`Stream` 优先 `ChatClient.Stream`。上游错误若是 `LiteLLMError`、取消或超时，直接返回，不用同步 `Chat` 兜底。其他流错误会再试一次非流式 `Chat`，并用 `singleMessageStream` 包成只含一条消息的 `StreamReader`（内部是 `schema.Pipe` 容量 1）。

流式 tool call 的发送片段：

```go
idx := tc.Index
sw.Send(&schema.Message{
    Role: schema.Assistant,
    ToolCalls: []schema.ToolCall{{
        Index:    &idx,
        ID:       id, // 尚未收到 id 时用 "pending"
        Type:     "function",
        Function: schema.FunctionCall{Name: tc.Name, Arguments: tc.ArgsDelta},
        Extra:    map[string]any{"delta": true},
    }},
}, nil)
```

发送给 Eino 的 `ID` 取自本地已累加的 `call.ID`。这一块还没有 id 时写成 `"pending"`。供应商若在第一块就带上 id，则 `call.ID` 在 `Send` 之前已经是真实 id，后续块复用它，`ConcatMessages` 可以合并。若第一块没有 id、后面的块才带来真实 id，同一 `Index` 上会先后出现 `"pending"` 和真实 id，`ConcatMessages` 返回 `cannot concat ToolCalls with different tool id`。本地 `calls[index]` 仍按 index 累加完整参数，供文本协议收尾使用；这条本地 map 不受上述 id 冲突影响。

usage 回调把 `PromptTokens`、`CompletionTokens`、`ReasoningTokens` 累加进 `RunResult`。不要在这里记录正文。

### 15.3 工具

只读工具在 `tools.go` 的 `buildTools`，全部 `utils.InferTool`：

| 常量 | 工具名 | 输入结构体 |
| --- | --- | --- |
| `ToolListDatabases` | `list_databases` | `emptyInput` |
| `ToolListCollections` | `list_collections` | `listCollectionsInput` |
| `ToolReadonlySQL` | `readonly_sql` | `readonlySQLInput` |
| `ToolListObjects` | `list_objects` | `listObjectsInput` |
| `ToolHeadObject` | `head_object` | `headObjectInput` |
| `ToolSearchLogs` | `search_logs` | `searchLogsInput` |
| `ToolLogLevelStats` | `log_level_stats` | 见 `tools.go` |
| `ToolSandboxExec` | `sandbox_exec` | `sandboxExecInput` |
| `ToolSandboxShell` | `sandbox_shell` | `sandboxShellInput` |
| `ToolSandboxReadFile` | `sandbox_read_file` | `sandboxReadFileInput` |
| `ToolSandboxWriteFile` | `sandbox_write_file` | `sandboxWriteFileInput` |

未知 id 被 `KnownTool` 丢掉。沙盒工具在 `Sandbox == nil` 或不可用时，`buildTools` 返回错误，而不是注册一个永远失败的工具。

每个工具从 `runContextFrom` 取 `RunContext`（`ProjectID`、`Principal`、`ThreadID`、`RunID`、`Skills`、`Headless`、`Emit`）。`StartRun` 在创建 Runner 之前 `withRunContext`。工具参数里没有 API key。

`wrapToolErrors` / `errorTolerantTool.InvokableRun`：成功原样返回；取消和超时原样返回；其他错误 `marshalToolJSON({"error","is_error":true})`。`StartRun` 用 `toolResultIsError` 给 SSE 打 `IsError`。

`SkillsCLI` 为真时不注册上述只读数据工具，改为 `buildSkillTools`：

```go
utils.InferTool("simplebase", "...", func(ctx context.Context, in simplebaseArgs) (string, error) {
    return r.execSimplebase(ctx, req, in.Argv)
})
```

`simplebaseArgs.Argv` 是字符串数组，不含二进制名。`execSimplebase` 先 `compose.GetToolCallID`，再 `callGate.wait(callID, 5s)`。`StartRun` 在发出 `tool_call` 事件后 `callGate.release`。这样 UI 先看到调用卡片，工具再执行。超时 5 秒仍会继续，避免模型没带上 id 时卡死。

argv 限制：1 到 64 段，单段不超过 32KiB，合计不超过 256KiB，禁止出现令牌字面量。`AllowArgv` 按本轮已加载的 skill 前缀放行（`skills.go` 的 `skillPrefixes`）。破坏性命令走 `ConfirmationWaiter`，headless 直接拒绝。

沙盒工具 id 即使在 CLI 模式下仍会附加到 `simplebase` 旁边，因为它们不走 HTTP CLI。

### 15.4 历史与事件

`historyToSchema` 从最近的消息往前装，预算 `maxHistoryChars`（12000）。单条截断到 2000 字。映射：

| 存储角色 | schema |
| --- | --- |
| assistant | 可选一条 `UserMessage("TOOL_HISTORY\n...")`，再 `AssistantMessage(content, nil)` |
| tool | `UserMessage("TOOL_RESULT\n"+content)` |
| system | `SystemMessage` |
| 其他 | `UserMessage` |

历史里的工具调用**没有**还原成 `ToolCall` + `ToolMessage` 配对。摘要是文本，避免旧的 tool_call id 和残缺参数把下一轮模型协议弄乱。当前轮真正的工具消息由 ADK 自己维护，不经过这个函数。

用户输入截断到 `maxUserChars`（8000），然后 `schema.UserMessage`。

`Event` 类型与发出位置：

| `Type` | 位置 |
| --- | --- |
| `token` | `consumeAssistant` 读到非工具文本 |
| `tool_call_delta` | `isToolDelta` 为真的 chunk，参数是本块增量 |
| `tool_call` | 流结束或非流式消息里出现完整 `ToolCalls` |
| `tool_result` | `MessageVariant.Role == schema.Tool` |
| `confirmation_required` / `confirmation_resolved` | `execSimplebase` |

`tool_result` 正文截断到 4000 个 rune，`Truncated` 标出。`ToolCallsJSON` 存在 `RunResult` 里，是卡片数组，不是模型原始协议。

`consumeAssistant` 在流式错误且已经拼到 tool call 时，仍返回 `schema.AssistantMessage("", merged)` 加该错误。`StartRun` 把错误当作 `runErr` 结束本轮，已经拼好的调用不会再执行，因为返回发生在 ADK 把消息送进工具节点之前——这里返回值只给 `StartRun` 用来发事件。真正执行工具的是 ADK 内部对同一条流的消费。两边对 delta 的理解必须一致，否则 UI 卡片和实际执行的参数会不一致。适配器侧保证每个 delta 都带 index 和 `Extra["delta"]`，UI 侧用 `mergeStreamToolCall`，ADK 内部用 `ConcatMessages`。

### 15.5 指令

`prompt.go` 的 `AssembleInstruction` 按顺序拼接模块模板（`ModuleTemplate`）、Agent 的 system prompt、项目环境、快照、以及 CLI 模式下的 `SkillPrompt`。`platformSkillsPrompt` 要求模型不要发明数据、不要输出密钥。`RedactSecrets` 在快照阶段遮住常见密钥形状。这些字符串是普通文本，不是 `schema.FString`，因此 JSON 示例里的大括号不会被 ADK 的默认 `GenModelInput` 误当成占位符——前提是指令里没有 `{Identifier}` 这种会被 f-string 识别的片段。若要在指令里写字面量花括号示例，先确认默认 `GenModelInput` 的报错信息（`chatmodel.go` 的 `defaultGenModelInput`：格式化失败时提示改用自定义 `GenModelInput`）。

### 15.6 和 Eino 能力的对应

| Eino 能力 | gocloudagent |
| --- | --- |
| `adk.NewChatModelAgent` + `NewRunner` | `StartRun` |
| `model.ToolCallingChatModel` | `gatewayChatModel` |
| `utils.InferTool` | `buildTools`、`buildSkillTools` |
| `compose.ToolsNodeConfig` | 只填 `Tools` |
| `compose.GetToolCallID` | CLI 确认与 `callGate` |
| `schema.Pipe` / `ConcatMessages` 的 Index 规则 | `gatewayChatModel.Stream` |
| `schema.UserMessage` 等 | `historyToSchema` |
| `adk.ErrExceedMaxIterations` | 转成 `max_iterations` 结果 |
| Chain / Graph / Workflow / react / host | 当前运行时未使用 |
| Checkpoint、`tool.StatefulInterrupt` | 当前运行时未使用；确认是自研 waiter |
| embedding / retriever | 未使用 |

扩展云 Agent 时，新工具优先 `InferTool` + 小结构体，返回 JSON 字符串，经 `wrapToolErrors` 注册进 `buildTools`，并在 `KnownTool` 与模块默认工具表登记。需要流式工具输出时再实现 `StreamableTool`，同时改 `consumeVariant` 的读者数量（流只能读一次）。不要在工具里打印 SQL 或对象内容到日志。

## 16. 最佳实践与常见问题

**工具 schema。** 用 `json` 加 `jsonschema:"required"` 加 `jsonschema_description`。描述写清何时调用、参数单位和只读限制。gocloudagent 现有 tag 把描述写在 `jsonschema:"description=..."`，与 `doc.go` 推荐的独立 tag 不同；新工具跟随 `doc.go`，避免逗号分隔被截断。

**WithTools 的身份。** 假模型和有内部游标的适配器，`WithTools` 应返回共享状态的指针（或显式拷贝游标）。`gatewayChatModel.WithTools` 值拷贝结构体后替换 `bound`，游标不在结构体里，拷贝是安全的。

**流式工具参数。** 每块带同一个 `Index`，name 只出现在第一块，arguments 只放增量，id 在到达前用稳定占位符，到达后的块必须带上同一个 id。用 `schema.ConcatMessages` 做单测。

**关闭流。** 谁 `Recv` 谁 `Close`。`Copy` 之后关闭副本。分支条件和 `StreamToolCallChecker` 读完模型流要关掉。

**错误文本与失败。** 想让模型继续，就返回字符串和 nil error。想停掉整轮，返回 `ctx.Err()`。未知工具用 `UnknownToolsHandler`，不要让幻觉名称变成 500。

**历史配对。** 送给 `ToolCallingChatModel` 的消息里，每个 `ToolCall` 都应有对应 `ToolMessage`。gocloudagent 把旧轮折成文本，是为了避开残缺 id；新写的直接模型调用不要模仿这种折法，除非同样不把它们放进原生 tools 协议。

**Instruction 里的花括号。** ADK 默认按 f-string 格式化 `Instruction`。不需要 session 插值时，提供一个原样拼接的 `GenModelInput`。

**Checkpoint 类型注册。** `schema.Register` 放 `init`。只注册作为 state 或 `any` 容器里的具体类型。未导出字段不会进快照。

**步数。** react 的 `MaxStep`、ADK 的 `MaxIterations`、compose 的 `WithMaxRunSteps` 是三套上限。gocloudagent 只设置 ADK 的 `MaxIterations`。

**回调与密钥。** `OnEnd` 能看到完整消息。生产 handler 只记 `RunInfo.Name`、耗时、token 数。不要 `AppendGlobalHandlers` 一个会打印正文的 handler。

**并行工具。** 默认并行。工具之间有顺序或共享文件时，`ToolsNodeConfig.ExecuteSequentially = true`。

**版本。** 业务代码以 v0.9.19 为准。v0.9.21 的 `CacheWriteTokens` 和流式工具取消 checkpoint 不要写进 v0.9.19 的结构体字面量。

常见现象：

| 现象 | 原因 |
| --- | --- |
| `tool not found` 且名字为空 | 增量 chunk 没有 `Index`，空 name 成了独立调用 |
| 参数 JSON 重复两遍 | 每块 `Arguments` 放的是累计全文 |
| `cannot concat ToolCalls with different tool id` | 同一 index 上先后出现了两个不同的非空 id。`gatewayChatModel` 在 id 未到时写入 `"pending"`，随后又写入真实 id 就会触发。占位符一旦写出，后续块要继续用它，或等第一块拿到真实 id 再发 |
| 流式只有第一句就去调工具 | `StreamToolCallChecker` 只看了第一块；模型先输出了文本 |
| `message placeholder format: history not found` | `MessagesPlaceholder` 的 key 缺失且 `optional` 为 false |
| checkpoint 恢复后状态是零值 | 类型没 `schema.Register`，或字段未导出 |
| `WithTools` 之后脚本模型立刻 `done` | 值拷贝导致游标丢失，或返回了新的空模型 |
| 工具执行了但 UI 没有卡片 | `Extra["delta"]` 没设，`consumeAssistant` 不会发 `tool_call_delta`；完整调用仍应在流结束时以 `tool_call` 发出 |
| 指令格式化失败 | `Instruction` 含 `{...}`，默认 `GenModelInput` 当 f-string |

## 17. 参考链接

源码均指 v0.9.19 树，除非另行说明。

Eino 核心：

- 仓库：<https://github.com/cloudwego/eino>
- tag：<https://github.com/cloudwego/eino/releases/tag/v0.9.19>
- `schema/message.go`：`Message`、`ConcatMessages`、`concatToolCalls`、`UserMessage`
- `schema/tool.go`：`ToolInfo`、`ParameterInfo`、`ToolChoice`
- `schema/stream.go`：`Pipe`、`StreamReader`、`Copy`、`MergeNamedStreamReaders`
- `schema/document.go`：`Document`
- `schema/serialization.go`：`Register`、`RegisterName`
- `components/model/interface.go`、`components/model/option.go`
- `components/tool/interface.go`、`components/tool/interrupt.go`
- `components/tool/utils/invokable_func.go`、`streamable_func.go`、`doc.go`
- `components/prompt/chat_template.go`
- `components/embedding/interface.go`、`indexer/interface.go`、`retriever/interface.go`、`document/interface.go`
- `compose/chain.go`、`chain_branch.go`、`chain_parallel.go`
- `compose/graph.go`：`START`、`END`、`AddLambdaNode`、`AddBranch`
- `compose/workflow.go`、`field_mapping.go`
- `compose/types_lambda.go`、`runnable.go`、`state.go`、`tool_node.go`（`GetToolCallID`）
- `compose/checkpoint.go`、`interrupt.go`、`resume.go`
- `callbacks/interface.go`、`handler_builder.go`、`aspect_inject.go`
- `utils/callbacks/template.go`
- `flow/agent/react/react.go`、`option.go`
- `flow/agent/multiagent/host/types.go`、`compose.go`
- `adk/interface.go`、`chatmodel.go`、`runner.go`、`interrupt.go`、`flow.go`、`workflow.go`
- `adk/retry_chatmodel.go`、`prebuilt/deep/deep.go`、`prebuilt/supervisor/supervisor.go`、`prebuilt/planexecute/plan_execute.go`

v0.9.21 相对 v0.9.19 的提交范围：`schema/message.go` 的 `CacheWriteTokens`，`compose/tool_node.go` 与 `compose/checkpoint.go` 的流式工具取消，`adk/cancel.go` 的 `StreamCanceledError.Is`。

示例仓库 <https://github.com/cloudwego/eino-examples>：

| 主题 | 路径 |
| --- | --- |
| Chain，含分支与并行 | `compose/chain` |
| Graph | `compose/graph/simple`、`state`、`async_node`、`tool_call_once`、`react_with_interrupt` |
| Workflow | `compose/workflow/1_simple` 至 `6_stream_field_map` |
| ReAct | `flow/agent/react` |
| Host 多 Agent | `flow/agent/multiagent/host` |
| ADK ChatModel | `adk/intro/chatmodel` |
| Transfer | `adk/intro/transfer` |
| 顺序 / 并行 / 循环 | `adk/intro/workflow` |
| Session | `adk/intro/session` |
| Human-in-the-loop | `adk/human-in-the-loop/1_approval` 至 `8_supervisor-plan-execute` |
| Deep / Supervisor | `adk/multiagent/deep`、`adk/multiagent/supervisor` |
| 流式 HTTP | `adk/intro/http-sse-service` |
| 快速开始 | `quickstart/chatwitheino`、`quickstart/eino_assistant` |

扩展实现 <https://github.com/cloudwego/eino-ext>：`components/model/{openai,ark,ollama,deepseek,claude,gemini,qwen}`，`components/tool/{httprequest,mcp,duckduckgo,wikipedia,googlesearch}`，`callbacks/{cozeloop,langfuse,langsmith}`。

本仓库：

- `internal/gocloudagent/runtime.go`：`StartRun`、`historyToSchema`、`consumeAssistant`、`mergeStreamToolCall`
- `internal/gocloudagent/model.go`：`gatewayChatModel`
- `internal/gocloudagent/tools.go`：`buildTools`、`wrapToolErrors`
- `internal/gocloudagent/cli_tool.go`：`buildSkillTools`、`callGate`、`execSimplebase`
- `internal/gocloudagent/skills.go`：skill 前缀与 `SkillPrompt` 的加载
- `internal/gocloudagent/prompt.go`：`AssembleInstruction`
- `internal/gocloudagent/modules.go`：工具名常量
