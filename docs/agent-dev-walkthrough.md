# 从零开发一个 Langchaingo Agent 应用：完整链路实战

> 本文以 git-agent 项目为真实案例，从一个初学者的视角，按**实际开发顺序**一步步讲解：从"什么都没有"到"一个能跑的 Agent"，每一行代码为什么这么写、每一步设计决策背后的思考是什么。读完本文，你将理解 Agent 应用的完整数据流，并能够复用这套模式开发自己的 AI + Anything 应用。

---

## 目录

1. [前置知识：Agent 基础概念](#前置知识agent-基础概念)
2. [前置知识：Langchaingo 框架入门](#前置知识langchaingo-框架入门)
3. [第一步：搞清楚你要做什么](#第一步搞清楚你要做什么)
4. [第二步：搭建项目骨架和依赖](#第二步搭建项目骨架和依赖)
5. [第三步：先实现不依赖 LLM 的本地模式（核心业务层）](#第三步先实现不依赖-llm-的本地模式核心业务层)
   - 5.1 [业务操作封装层 (gitwrapper)](#31-业务操作封装层-gitwrapper)
   - 5.2 [意图解析器 (interpreter) —— 让程序"听懂"用户的话](#32-意图解析器-interpreter--让程序听懂用户的话)
   - 5.3 [执行规划器 (planner) —— 把意图变成可执行的步骤列表](#33-执行规划器-planner--把意图变成可执行的步骤列表)
   - 5.4 [冲突检测器 (conflict) —— 提前发现文件冲突](#34-冲突检测器-conflict--提前发现文件冲突)
   - 5.5 [仓库管理器 (repository) —— 管理多个仓库](#35-仓库管理器-repository--管理多个仓库)
   - 5.6 [Agent 核心 (agent) —— 串起感知→推理→行动→反馈的循环](#36-agent-核心-agent--串起感知推理行动反馈的循环)
   - 5.7 [主程序入口 (main.go) —— 让用户能交互使用](#37-主程序入口-maingo--让用户能交互使用)
6. [第四步：引入 LLM，让 Agent 变"聪明"](#第四步引入-llm让-agent-变聪明)
   - 6.1 [LLM 接入层 (llm/langchain.go) —— 连接大模型](#41-llm-接入层-llmlangchaingo--连接大模型)
   - 6.2 [工具定义 (llm/tools.go) —— 告诉 LLM 它能做什么](#42-工具定义-llmtoolsgo--告诉-llm-它能做什么)
   - 6.3 [工具注册中心 (llm/git_tools.go) —— 把定义和执行绑在一起](#43-工具注册中心-llmgit_toolsgo--把定义和执行绑在一起)
   - 6.4 [系统提示词 (agent.go buildSystemPrompt) —— 告诉 LLM 它是谁](#44-系统提示词-agentgo-buildsystemprompt--告诉-llm-它是谁)
   - 6.5 [Agent 的 LLM 模式 —— ReAct 循环](#45-agent-的-llm-模式--react-循环)
7. [第五步：动态提示词系统 (promptkit) —— 让 Agent 可配置、可热更新](#第五步动态提示词系统-promptkit--让-agent-可配置可热更新)
8. [第六步：完整数据流追踪 —— 一条用户消息从头到尾经历了什么](#第六步完整数据流追踪--一条用户消息从头到尾经历了什么)
9. [通用 Agent 开发模板 —— AI + Anything 复用清单](#通用-agent-开发模板--ai--anything-复用清单)
10. [踩坑经验与注意事项](#踩坑经验与注意事项)
11. [调试和测试方法](#调试和测试方法)
12. [进阶功能扩展](#进阶功能扩展)
13. [参考资源](#参考资源)

---

## 前置知识：Agent 基础概念

### 什么是 AI Agent？

AI Agent（智能代理）是一个能够：
- **感知环境**：接收用户输入和系统状态
- **思考决策**：使用 LLM 理解意图并制定计划（或通过本地关键词匹配）
- **执行行动**：调用工具函数完成具体任务
- **学习改进**：根据执行结果调整策略

### 双模式架构对比

git-agent 采用**双模式架构**：本地模式 + LLM 模式。这是关键设计：

| 特性 | 本地模式 | LLM 模式 |
|------|---------|----------|
| 意图解析 | 关键词匹配（Interpreter） | LLM Function Calling |
| 执行规划 | Planner 映射 | LLM 自动决策 |
| 工具调用 | Agent.executeStep | ReAct 循环 |
| 稳定性 | ⭐⭐⭐⭐⭐ 极高 | ⭐⭐⭐ 依赖 API |
| 智能程度 | ⭐⭐ 有限 | ⭐⭐⭐⭐⭐ 极强 |
| 降级能力 | — | LLM 失败时自动降级到本地 |

> **核心原则**：先实现本地模式，再引入 LLM 增强。本地模式是保底，LLM 是锦上添花。

### 本地模式 vs LLM 解析意图对比

| 对比 | 关键词匹配 | LLM 解析 |
|------|-----------|---------|
| 延迟 | <1ms | 500-2000ms |
| 成本 | 0 | 每次 ~0.01元 |
| 稳定性 | 极高 | 依赖 API |
| 可解释性 | 100% | 黑盒 |
| 灵活性 | 有限 | 极强 |

**实际策略**：本地 Interpreter 做快速预判（用于工具筛选），LLM 做精细理解。两者配合使用。

---

## 前置知识：Langchaingo 框架入门

### 什么是 Langchaingo？

[Langchaingo](https://github.com/tmc/langchaingo) 是 LangChain 的 Go 语言实现，提供了：
- 统一的 LLM 接口（支持 OpenAI、Azure、Ollama 等所有 OpenAI 兼容 API）
- 工具调用（Function Calling / Tool Calling）机制
- 对话历史管理（MessageContent）
- 链式调用（Chains）支持

### 安装

```bash
go get github.com/tmc/langchaingo@v0.1.14
```

### 基础使用：创建 LLM 实例

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/tmc/langchaingo/llms"
    "github.com/tmc/langchaingo/llms/openai"
)

func main() {
    // 创建 LLM 实例（支持所有 OpenAI 兼容 API）
    llm, err := openai.New(
        openai.WithToken("your-api-key"),
        openai.WithModel("gpt-4o"),
        openai.WithBaseURL("https://api.openai.com/v1"),  // 可换成 DeepSeek、通义千问、Ollama 等
    )
    if err != nil {
        log.Fatal(err)
    }
    
    ctx := context.Background()
    response, err := llm.Call(ctx, "什么是 AI Agent？")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(response)
}
```

### Function Calling（工具调用）完整示例

这是 Agent 开发中最核心的交互模式——让 LLM 能够调用你定义的工具函数：

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/tmc/langchaingo/llms"
    "github.com/tmc/langchaingo/llms/openai"
)

// 1. 定义工具（告诉 LLM 它能调用什么）
func getWeatherTool() llms.Tool {
    return llms.Tool{
        Type: "function",
        Function: &llms.FunctionDefinition{
            Name:        "get_weather",
            Description: "获取指定城市的天气信息",
            Parameters: map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "city": map[string]interface{}{
                        "type":        "string",
                        "description": "城市名称，如：北京、上海",
                    },
                },
                "required": []string{"city"},
            },
        },
    }
}

// 2. 实现工具执行函数
func executeGetWeather(arguments map[string]interface{}) string {
    city := arguments["city"].(string)
    return fmt.Sprintf("%s 的天气：晴天，温度 25°C", city)
}

func main() {
    llm, _ := openai.New(
        openai.WithToken("your-api-key"),
        openai.WithModel("gpt-4o"),
    )
    ctx := context.Background()
    
    // 3. 准备消息
    messages := []llms.MessageContent{
        llms.TextParts(llms.ChatMessageTypeSystem, "你是一个天气助手"),
        llms.TextParts(llms.ChatMessageTypeHuman, "北京今天天气怎么样？"),
    }
    
    // 4. 调用 LLM，启用工具调用
    response, err := llm.GenerateContent(
        ctx, messages,
        llms.WithTools(getWeatherTool()),
        llms.WithToolChoice("auto"),
        llms.WithTemperature(0.1),
    )
    if err != nil {
        log.Fatal(err)
    }
    
    // 5. 处理工具调用请求
    choice := response.Choices[0]
    if len(choice.ToolCalls) > 0 {
        tc := choice.ToolCalls[0]
        fmt.Printf("LLM 请求调用工具: %s, 参数: %v\n", tc.FunctionCall.Name, tc.FunctionCall.Arguments)
        
        // 6. 执行工具
        result := executeGetWeather(tc.FunctionCall.Arguments)
        fmt.Printf("工具执行结果: %s\n", result)
        
        // 7. 将工具结果回传 LLM，让它生成最终回复
        messages = append(messages, llms.MessageContent{
            Role: llms.ChatMessageTypeAI,
            Parts: []llms.ContentPart{llms.ToolCall{
                ID:   tc.ID,
                Type: "function",
                FunctionCall: &llms.FunctionCall{
                    Name:      tc.FunctionCall.Name,
                    Arguments: tc.FunctionCall.Arguments,
                },
            }},
        })
        messages = append(messages, llms.MessageContent{
            Role: llms.ChatMessageTypeTool,
            Parts: []llms.ContentPart{llms.ToolCallResponse{
                ToolCallID: tc.ID,
                Name:       tc.FunctionCall.Name,
                Content:    result,
            }},
        })
        
        finalResp, _ := llm.GenerateContent(ctx, messages)
        fmt.Printf("最终回复: %s\n", finalResp.Choices[0].Content)
    }
}
```

> **核心理解**：Function Calling 的流程是 `定义工具 → LLM 决定调用 → 你执行工具 → 结果回传 LLM → LLM 生成最终回复`。这就是 Agent 的"行动"能力来源。

---

## 第一步：搞清楚你要做什么

在写任何代码之前，先回答三个问题：

| 问题 | git-agent 的回答 |
|------|-----------------|
| **目标用户是谁？** | 不懂技术的办公人员，不会用 git 命令 |
| **用户能用什么方式交互？** | 自然语言对话，比如"保存修改"、"看看小李改了什么" |
| **Agent 需要能做什么？** | 保存版本、查看历史、恢复版本、查看差异、团队协作… |

想清楚之后，你会得到一张**意图列表**：

```
save_version     — 用户想保存当前修改
view_history     — 用户想看修改记录
restore_version  — 用户想回到之前的版本
view_diff        — 用户想看改了什么
view_status      — 用户想看当前状态
submit_change    — 用户想把修改推送给同事
view_team_change — 用户想看团队成员的修改
push             — 用户想推送到远程
pull             — 用户想拉取最新内容
create_branch    — 用户想创建工作副本
switch_branch    — 用户想切换工作副本
list_branches    — 用户想列出所有工作副本
create_tag       — 用户想打标签
detect_conflict  — 用户想检测冲突
resolve_conflict — 用户想解决冲突
init_repo        — 用户想初始化仓库
update_user_info — 用户想设置名字/邮箱
help             — 用户需要帮助
```

> **关键认知**：每一个"意图"背后，都对应一组具体的**操作步骤**。Agent 的工作就是把用户的自然语言翻译成这些操作步骤，然后执行。

---

## 第二步：搭建项目骨架和依赖

### 2.1 项目结构

```
git-agent/
├── main.go                     # 程序入口（交互式 CLI）
├── Makefile                    # 构建脚本（注入版本信息）
├── go.mod                      # 依赖管理
├── internal/
│   ├── version.go              # 版本信息（通过 -ldflags 注入）
│   ├── agent/
│   │   └── agent.go            # Agent 核心（状态管理 + 本地模式 + LLM 模式 + ReAct 循环）
│   │   └── agent_test.go       # Agent 单元测试
│   ├── interpreter/
│   │   └── interpreter.go      # 意图解析（自然语言 → 结构化意图，纯关键词匹配）
│   │   └── interpreter_test.go # 意图解析单元测试
│   │   └── verify_test.go      # 验证测试
│   ├── planner/
│   │   └── planner.go          # 执行规划（意图 → 步骤列表）
│   │   └── planner_test.go     # 规划器单元测试
│   ├── gitwrapper/
│   │   └── gitwrapper.go       # 业务操作封装（git 命令的 Go 封装）
│   ├── llm/
│   │   ├── langchain.go        # LLM 接入层（OpenAI 兼容 API 配置 + 实例创建）
│   │   ├── tools.go            # 工具定义（JSON Schema，告诉 LLM 能调用什么）
│   │   └── git_tools.go        # 工具注册中心（GitAgentTool → LangChain Tool 适配 + 执行器映射）
│   ├── promptkit/
│   │   ├── embed.go            # 嵌入资源文件声明（//go:embed resources）
│   │   ├── promptkit.go        # Skills/Rules 动态加载 + 意图索引 + 热更新
│   │   ├── promptkit_test.go   # promptkit 单元测试
│   │   └── resources/
│   │       ├── skills/         # 技能（操作知识）
│   │       │   ├── batch-commit.md          # 分批提交技能
│   │       │   ├── conflict-resolution.md   # 冲突解决技能
│   │       │   └── push-fail-guide.md       # 推送失败引导技能
│   │       └── rules/          # 规则（行为约束）
│   │           ├── always-execute.md        # 直接执行规则
│   │           ├── commit-message.md        # Commit message 规范
│   │           ├── display-format.md        # 显示格式规则
│   │           ├── no-git-terms.md          # 术语翻译规则
│   │           ├── no-repeat-tools.md       # 不重复调用工具规则
│   │           └── version-restore.md       # 版本恢复确认规则
│   ├── conflict/
│   │   └── conflict.go         # 冲突检测与解决（edit-edit、edit-delete、add-add）
│   │   └── conflict_test.go    # 冲突检测单元测试
│   └── repository/
│       └── repository.go       # 仓库管理器（创建、克隆、列表）
```

### 2.2 核心依赖

```
// go.mod
module github.com/jackz-jones/git-agent

go 1.24.4

require (
    github.com/tmc/langchaingo v0.1.14   // LangChain Go 框架（LLM 调用 + Function Calling）
    github.com/go-git/go-git/v5 v5.18.0  // Git 操作库（按业务替换为你的业务 SDK）
    github.com/chzyer/readline v1.5.1    // 交互式命令行（自动补全 + 历史记录）
    github.com/fsnotify/fsnotify v1.9.0  // 文件监听（Skills/Rules 热加载）
)
```

> **为什么选 langchaingo？** 它是 LangChain 的 Go 语言实现，提供了 LLM 调用、Function Calling（工具调用）、对话历史管理等核心能力。你不需要自己拼 HTTP 请求，框架帮你封装好了。

### 2.3 核心模块职责总览

| 模块 | 文件 | 职责 | 关键导出 |
|------|------|------|---------|
| **agent** | `agent/agent.go` | Agent 核心控制器：状态管理、双模式调度、ReAct 循环 | `Agent`, `AgentResponse`, `UserConfig`, `LLMConfig` |
| **interpreter** | `interpreter/interpreter.go` | 本地意图解析：纯关键词匹配，不依赖 LLM | `Interpreter`, `UserIntent`, `IntentType` |
| **planner** | `planner/planner.go` | 执行规划：意图 → 步骤列表 | `Planner`, `Plan`, `Step`, `StepType` |
| **gitwrapper** | `gitwrapper/gitwrapper.go` | 业务操作封装：git 命令的 Go 函数 | `GitWrapper`, `VersionInfo` |
| **llm** | `llm/langchain.go` | LLM 接入层：OpenAI 兼容 API 配置 + 实例创建 | `OpenAIConfig`, `Usage`, `NewLangChainLLM()` |
| **llm** | `llm/tools.go` | 工具定义：JSON Schema 格式，告诉 LLM 能调用什么 | `GitAgentTool`, `AllGitAgentTools` |
| **llm** | `llm/git_tools.go` | 工具注册中心：定义↔执行器适配 | `GitToolRegistry`, `GitTool`, `NewGitToolRegistry()` |
| **promptkit** | `promptkit/promptkit.go` | Skills/Rules 动态加载 + 意图索引 + 热更新 | `Kit`, `Config`, `NewKit()` |
| **promptkit** | `promptkit/embed.go` | 嵌入资源文件声明 | `ResourcesFS` |
| **conflict** | `conflict/conflict.go` | 冲突检测与解决建议 | `Detector`, `FileConflict`, `ConflictType` |
| **repository** | `repository/repository.go` | 仓库管理器 | `Manager`, `RepoInfo` |
| **version** | `version.go` | 版本信息（Makefile -ldflags 注入） | `Version`, `BuildTime`, `CommitID` |

---

## 第三步：先实现不依赖 LLM 的本地模式（核心业务层）

> **核心理念**：先让 Agent 在没有 LLM 的情况下也能工作。这样你有一个可用的基线，即使 LLM 挂了也能降级运行。

本地模式的数据流是这样的：

```
用户输入 "保存修改"
    │
    ▼
Interpreter.Parse() ──→ UserIntent{Type: "save_version", Params: {"message":"保存修改"}}
    │
    ▼
Planner.CreatePlan() ──→ Plan{Steps: [{git_add}, {git_commit}]}
    │
    ▼
Agent.executePlan() ──→ 逐步执行 git_add、git_commit
    │
    ▼
Interpreter.TranslateResult() ──→ "✅ 已保存为新版本 #abc1234"
```

### 3.1 业务操作封装层 (gitwrapper)

**这一层做什么？** 把你的业务操作封装成 Go 函数。在 git-agent 里就是 git 操作，在你的项目里可能是发邮件、操作数据库、调 API 等等。

```go
// internal/gitwrapper/gitwrapper.go（简化示例）

type GitWrapper struct {
    repoPath string
    repo     *git.Repository
}

// SaveVersion 保存一个新版本（add + commit）
func (g *GitWrapper) SaveVersion(message string, files []string, name, email string) (map[string]string, error) {
    // 1. add
    if len(files) > 0 {
        g.AddFiles(files)
    } else {
        g.AddAll()
    }
    // 2. commit
    hash, err := g.Commit(message, name, email)
    if err != nil {
        return nil, err
    }
    return map[string]string{"hash": hash}, nil
}

// GetHistory 获取版本历史
func (g *GitWrapper) GetHistory(file string, limit int, author string) ([]VersionInfo, error) {
    // ...遍历 git log，返回结构化数据
}
```

> **为什么要单独一层？** 因为 Agent 层不应该关心"怎么执行 git 命令"这种细节。Agent 只需要调用 `SaveVersion()`，至于底层是用 go-git 库还是调用 git 命令行，Agent 不需要知道。这种分层让你可以独立测试、独立替换底层实现。

### 3.2 意图解析器 (interpreter) —— 让程序"听懂"用户的话

**这一层做什么？** 把用户的自然语言输入，翻译成结构化的意图对象。**注意：git-agent 的 interpreter 是纯本地关键词匹配，不依赖 LLM。**

```go
// internal/interpreter/interpreter.go（关键结构）

// IntentType 意图类型的枚举
type IntentType string
const (
    IntentSaveVersion    IntentType = "save_version"
    IntentViewHistory    IntentType = "view_history"
    IntentRestoreVersion IntentType = "restore_version"
    IntentViewDiff       IntentType = "view_diff"
    IntentViewStatus     IntentType = "view_status"
    IntentSubmitChange   IntentType = "submit_change"
    IntentViewTeamChange IntentType = "view_team_change"
    IntentPush           IntentType = "push"
    IntentPull           IntentType = "pull"
    IntentCreateBranch   IntentType = "create_branch"
    IntentSwitchBranch   IntentType = "switch_branch"
    IntentListBranches   IntentType = "list_branches"
    IntentCreateTag      IntentType = "create_tag"
    IntentDetectConflict IntentType = "detect_conflict"
    IntentInitRepo       IntentType = "init_repo"
    IntentUpdateUserInfo IntentType = "update_user_info"
    IntentHelp           IntentType = "help"
)

// UserIntent 解析后的用户意图
type UserIntent struct {
    Type        IntentType         `json:"type"`
    Target      string             `json:"target"`       // 操作目标（文件名、分支名等）
    UserInput   string             `json:"description"`  // 用户原始描述
    Params      map[string]string  `json:"params"`       // 附加参数
    Confidence  float64            `json:"confidence"`   // 解析置信度 0-1
}
```

**怎么解析？** 用关键词匹配（本地模式），这是最简单可靠的方式：

```go
// 每个意图对应一组关键词
type intentPattern struct {
    intentType IntentType
    keywords   []string
    extractor  func(input string, matches []string) *UserIntent
}

// 初始化时注册所有模式
i.patterns = []intentPattern{
    {
        intentType: IntentSaveVersion,
        keywords: []string{"保存", "提交", "存一下", "commit", "save", "保存修改", ...},
        extractor: extractSaveVersion,  // 从输入中提取参数
    },
    {
        intentType: IntentViewHistory,
        keywords: []string{"历史", "记录", "log", "history", "谁改的", ...},
        extractor: extractViewHistory,
    },
    // ...
}

// Parse 核心解析逻辑
func (i *Interpreter) Parse(input string) (*UserIntent, error) {
    // 策略1：精确匹配（输入完全等于某个关键词）
    // 策略2：模糊匹配（输入包含关键词，按匹配度评分）
    // 选出得分最高的意图
}
```

**参数提取器** 是关键细节，它从用户的输入中提取出操作需要的参数：

```go
// 用户说 "保存修改，添加了登录功能"
func extractSaveVersion(input string, matches []string) *UserIntent {
    intent := &UserIntent{Params: make(map[string]string)}
    // 把关键词从输入中去掉，剩下的就是描述信息
    desc := input
    for _, kw := range matches {
        desc = strings.ReplaceAll(desc, kw, "")
    }
    desc = strings.TrimSpace(desc)
    if desc != "" {
        intent.Params["message"] = desc  // "添加了登录功能"
    }
    return intent
}

// 用户说 "我的名字是张三，邮箱是 zhangsan@example.com"
func extractUpdateUserInfo(input string, matches []string) *UserIntent {
    intent := &UserIntent{Params: make(map[string]string)}
    // 提取邮箱（包含 @ 的部分）
    for _, p := range strings.Fields(input) {
        if strings.Contains(p, "@") {
            intent.Params["email"] = p
        }
    }
    // 提取名字（"我叫XXX" 或 "我的名字是XXX"）
    for _, prefix := range []string{"我叫", "我的名字是", "名字"} {
        if idx := strings.Index(input, prefix); idx >= 0 {
            remaining := strings.TrimSpace(input[idx+len(prefix):])
            if parts := strings.Fields(remaining); len(parts) > 0 {
                intent.Params["name"] = parts[0]
            }
            break
        }
    }
    return intent
}
```

> **初学者常见疑问：关键词匹配是不是太简陋了？** 对于有限意图集合（十几个到几十个），关键词匹配实际上非常稳定可靠。LLM 模式是锦上添花，本地模式是你的保底。

### 3.3 执行规划器 (planner) —— 把意图变成可执行的步骤列表

**这一层做什么？** 有些意图需要多步操作。比如"提交给团队"需要 add → commit → push 三步。Planner 把意图映射成步骤列表。

```go
// internal/planner/planner.go（关键结构）

type StepType string
const (
    StepGitInit         StepType = "git_init"
    StepGitAdd          StepType = "git_add"
    StepGitCommit       StepType = "git_commit"
    StepGitPush         StepType = "git_push"
    StepGitPull         StepType = "git_pull"
    StepGitLog          StepType = "git_log"
    StepGitDiff         StepType = "git_diff"
    StepGitStatus       StepType = "git_status"
    StepGitBranch       StepType = "git_branch"
    StepGitMerge        StepType = "git_merge"
    StepGitTag          StepType = "git_tag"
    StepGitRestore      StepType = "git_restore"
    StepConflictDetect  StepType = "conflict_detect"
    StepConflictResolve StepType = "conflict_resolve"
    StepRepoCreate      StepType = "repo_create"
    StepRepoClone       StepType = "repo_clone"
    StepUpdateUserInfo  StepType = "update_user_info"
)

type Step struct {
    Type     StepType          `json:"type"`
    Params   map[string]string `json:"params"`
    Required bool              `json:"required"`    // 失败时是否终止整个计划
    Desc     string            `json:"description"` // 步骤描述
}

type Plan struct {
    Intent     *interpreter.UserIntent `json:"intent"`
    Steps      []*Step                 `json:"steps"`
    TotalSteps int                     `json:"total_steps"`
}

func (p *Planner) CreatePlan(intent *interpreter.UserIntent) (*Plan, error) {
    plan := &Plan{Intent: intent}
    switch intent.Type {
    case interpreter.IntentSaveVersion:
        // 保存版本：add → commit
        plan.Steps = []*Step{
            {Type: StepGitAdd, Required: true, Desc: "添加文件到暂存区"},
            {Type: StepGitCommit, Params: map[string]string{"message": intent.Params["message"]}, Required: true, Desc: "保存版本"},
        }
    case interpreter.IntentSubmitChange:
        // 提交给团队：add → commit → push
        plan.Steps = []*Step{
            {Type: StepGitAdd, Required: true, Desc: "添加所有修改"},
            {Type: StepGitCommit, Required: true, Desc: "提交修改"},
            {Type: StepGitPush, Required: true, Desc: "推送到远程"},
        }
    case interpreter.IntentViewStatus:
        // 查看状态：只需一步
        plan.Steps = []*Step{
            {Type: StepGitStatus, Required: true, Desc: "查看当前状态"},
        }
    }
    plan.TotalSteps = len(plan.Steps)
    return plan, nil
}
```

> **为什么要 Planner 而不是直接在 Agent 里 switch-case？** 因为 Planner 让"做什么"和"怎么做"分离。Agent 只需要拿到步骤列表依次执行，不需要关心每个意图具体需要哪些步骤。以后新增意图，只需在 Planner 里加一个 case。

### 3.4 冲突检测器 (conflict) —— 提前发现文件冲突

**这一层做什么？** 在多人协作场景中，不同人修改同一文件会产生冲突。conflict 模块负责检测和提供解决建议。

```go
// internal/conflict/conflict.go（关键结构）

type ConflictType string
const (
    ConflictEditEdit   ConflictType = "edit_edit"    // 双方修改了同一文件的同一部分
    ConflictEditDelete ConflictType = "edit_delete"  // 一方修改另一方删除
    ConflictAddAdd     ConflictType = "add_add"      // 双方添加了同名文件
)

type FileConflict struct {
    FilePath       string                `json:"file_path"`
    ConflictType   ConflictType          `json:"conflict_type"`
    OurChange      string                `json:"our_change"`
    TheirChange    string                `json:"their_change"`
    AutoResolvable bool                  `json:"auto_resolvable"`
    Resolution     *ResolutionSuggestion `json:"resolution,omitempty"`
}

type Detector struct {
    repoPath string
}
```

> **为什么冲突检测要独立模块？** 因为冲突检测涉及文件比较、合并策略等复杂逻辑，与 Agent 核心循环无关。独立模块方便测试和扩展策略。

### 3.5 仓库管理器 (repository) —— 管理多个仓库

**这一层做什么？** 提供仓库的创建、克隆、列表等管理能力。

```go
// internal/repository/repository.go（关键结构）

type RepoInfo struct {
    Name      string    `json:"name"`
    Path      string    `json:"path"`
    CreatedAt time.Time `json:"created_at"`
    IsBare    bool      `json:"is_bare"`
}

type Manager struct {
    basePath string
}

func (m *Manager) Create(name string) (*RepoInfo, error)  // 创建新仓库（含初始提交）
func (m *Manager) Clone(url string) (*RepoInfo, error)    // 克隆远程仓库
func (m *Manager) List() ([]RepoInfo, error)               // 列出所有仓库
```

### 3.6 Agent 核心 (agent) —— 串起感知→推理→行动→反馈的循环

**这是整个应用的大脑。** 它管理状态、串联各组件、处理异常。

```go
// internal/agent/agent.go（关键结构）

// Agent 状态
type AgentState string
const (
    StateIdle        AgentState = "idle"        // 空闲
    StateThinking    AgentState = "thinking"    // 正在理解意图
    StatePlanning    AgentState = "planning"    // 正在规划步骤
    StateExecuting   AgentState = "executing"   // 正在执行操作
    StateConflicting AgentState = "conflicting" // 遇到冲突
    StateError       AgentState = "error"       // 出错
)

// Agent 响应
type AgentResponse struct {
    Success     bool        `json:"success"`
    Message     string      `json:"message"`           // 用户友好的消息
    Details     string      `json:"details,omitempty"` // 技术细节
    State       AgentState  `json:"state"`
    Data        interface{} `json:"data,omitempty"`
    Suggestions []string    `json:"suggestions,omitempty"` // 后续建议
    Timestamp   time.Time   `json:"timestamp"`
    UsedLLM     bool        `json:"used_llm"`              // 本次响应是否使用了 LLM
    TokenUsage  *llm.Usage  `json:"token_usage,omitempty"` // LLM token 使用量
}

// Agent 配置
type UserConfig struct {
    Name     string `json:"name"`
    Email    string `json:"email"`
    Role     string `json:"role"`     // admin, editor, viewer
    Language string `json:"language"` // zh, en
}

type LLMConfig struct {
    Enabled   bool   `json:"enabled"`
    APIKey    string `json:"api_key"`
    BaseURL   string `json:"base_url"`
    Model     string `json:"model"`
    MaxTokens int    `json:"max_tokens"`
}

type Agent struct {
    // 本地模式组件
    gitWrapper       *gitwrapper.GitWrapper
    interpreter      *interpreter.Interpreter
    planner          *planner.Planner
    conflictDetector *conflict.Detector
    repoManager      *repository.Manager

    // LLM 模式组件
    langchainLLM llms.Model            // LangChain LLM 实例
    chatHistory  []llms.MessageContent // 对话上下文
    toolDefs     []llms.Tool           // 全量工具定义
    currentTools []llms.Tool           // 当前轮次智能筛选后的工具子集
    toolRegistry *llm.GitToolRegistry  // 工具注册中心
    promptKit    *promptkit.Kit        // Skills/Rules 按需加载器

    // 状态
    state      AgentState
    userConfig *UserConfig
    llmConfig  *LLMConfig
}
```

**两种创建方式：**

```go
// 本地模式：不需要 LLM
func New(repoPath string, userConfig *UserConfig) (*Agent, error)

// LLM 模式：初始化 LLM + 工具注册 + promptkit
func NewWithLLM(repoPath string, userConfig *UserConfig, llmConfig *LLMConfig) (*Agent, error)
```

**本地模式的主流程：**

```go
// Process —— Agent 的主入口
func (a *Agent) Process(ctx context.Context, input string) *AgentResponse {
    if a.IsLLMEnabled() {
        return a.processWithLLM(ctx, input)  // LLM 模式
    }
    return a.processLocal(ctx, input)  // 本地模式
}

// processLocal 本地模式的完整流程
func (a *Agent) processLocal(ctx context.Context, input string) *AgentResponse {
    // 第一步：感知 —— 解析用户意图
    a.setState(StateThinking)
    intent, err := a.interpreter.Parse(input)
    if err != nil {
        a.setState(StateError)
        return &AgentResponse{
            Success: false,
            Message: fmt.Sprintf("抱歉，我没能理解您的意思：「%s」", input),
            Suggestions: []string{"保存当前修改", "查看修改历史"},
        }
    }

    // 第二步：推理 —— 根据意图规划步骤
    a.setState(StatePlanning)
    plan, err := a.planner.CreatePlan(intent)

    // 第三步：行动 —— 逐步执行
    a.setState(StateExecuting)
    result, err := a.executePlan(ctx, plan)

    // 第四步：反馈 —— 翻译为用户友好的语言
    a.setState(StateIdle)
    return a.formatResponse(intent, result)
}
```

**executePlan —— 逐步执行计划：**

```go
func (a *Agent) executePlan(ctx context.Context, plan *planner.Plan) (*planner.ExecutionResult, error) {
    result := planner.NewExecutionResult()
    for i, step := range plan.Steps {
        stepResult, err := a.executeStep(step)
        if err != nil {
            result.AddFailedStep(i, step, err)
            if step.Required {
                return result, err  // 必要步骤失败，终止计划
            }
            continue  // 非必要步骤失败，继续
        }
        result.AddCompletedStep(i, step, stepResult)
    }
    return result, nil
}

// executeStep 根据步骤类型调用对应的业务操作
func (a *Agent) executeStep(step *planner.Step) (interface{}, error) {
    switch step.Type {
    case planner.StepGitAdd:
        return nil, a.gitWrapper.AddAll()
    case planner.StepGitCommit:
        return nil, a.gitWrapper.Commit(step.Params["message"], ...)
    case planner.StepGitPush:
        return nil, a.gitWrapper.Push(step.Params["remote"])
    // ...
    }
}
```

> **为什么需要状态管理？** 状态让你能：1）给用户显示 Agent 在做什么（"正在执行操作…"）；2）在冲突/错误状态下做出不同处理；3）方便调试。状态机是 Agent 的骨架。

### 3.7 主程序入口 (main.go) —— 让用户能交互使用

```go
// main.go 是根目录下的入口文件（不是 cmd/agent/main.go）

func main() {
    // 解析命令行参数
    llmAPIKey := flag.String("api-key", "", "LLM API Key")
    llmBaseURL := flag.String("base-url", "", "LLM API Base URL")
    llmModel := flag.String("model", "", "LLM 模型名称")
    repoPath := flag.String("repo", ".", "仓库路径")
    showVersion := flag.Bool("version", false, "显示版本")

    flag.Parse()

    if *showVersion {
        fmt.Println(internal.VersionInfo())  // 版本信息通过 -ldflags 注入
        return
    }

    // 构建 LLM 配置（优先级：命令行参数 > 环境变量）
    llmConfig := buildLLMConfig(apiKey, baseURL, model)

    // 创建 Agent（自动选择本地模式或 LLM 模式）
    userConfig := &agent.UserConfig{
        Name:     os.Getenv("GIT_AGENT_USER"),
        Email:    os.Getenv("GIT_AGENT_EMAIL"),
        Role:     "editor",
        Language: "zh",
    }

    if llmConfig.Enabled {
        a, err = agent.NewWithLLM(repoPath, userConfig, llmConfig)
    } else {
        a, err = agent.New(repoPath, userConfig)
    }

    // 启动交互式输入循环
    reader, _ := readline.NewEx(&readline.Config{
        Prompt:       "[local] > ",
        HistoryFile:  "/tmp/git-agent-history.tmp",
        AutoComplete: completer,  // /mode, /clear, exit 等命令补全
    })

    for {
        // 根据模式动态更新提示符
        modeIndicator := "local"
        if a.IsLLMEnabled() {
            modeIndicator = "llm"
        }
        reader.SetPrompt(fmt.Sprintf("[%s] > ", modeIndicator))

        line, _ := reader.Readline()
        input := strings.TrimSpace(line)

        // 内置命令处理
        if input == "exit" || input == "退出" { break }
        if strings.HasPrefix(input, "/mode") { handleModeSwitch(a, input); continue }
        if input == "/clear" { a.ClearConversation(); continue }

        // 通过 Agent 处理
        response := a.Process(ctx, input)
        printResponse(response)
    }

    a.Close()
}
```

**环境变量配置：**

| 环境变量 | 说明 | 必填 |
|---------|------|------|
| `GIT_AGENT_API_KEY` | LLM API Key | LLM 模式必填 |
| `GIT_AGENT_BASE_URL` | LLM API 地址 | 否 |
| `GIT_AGENT_MODEL` | LLM 模型名称 | 否 |
| `GIT_AGENT_MAX_TOKENS` | 最大 token 数 | 否（默认 4096） |
| `GIT_AGENT_USER` | 用户名 | 是 |
| `GIT_AGENT_EMAIL` | 用户邮箱 | 是 |
| `GIT_HTTP_USERNAME` | HTTPS 认证用户名 | 推送时需要 |
| `GIT_HTTP_PASSWORD` | HTTPS 认证密码/令牌 | 推送时需要 |

> **到这一步，你已经有一个能用的 Agent 了！** 虽然它只能通过关键词理解意图，但功能完整、稳定可靠。接下来引入 LLM 是让 Agent 从"能用"变"好用"。

---

## 第四步：引入 LLM，让 Agent 变"聪明"

> LLM 模式的核心思想：**让 LLM 替代 interpreter + planner 的工作**。用户说任何话，LLM 都能理解，并自动决定该调用什么工具。

LLM 模式的数据流：

```
用户输入 "保存修改"
    │
    ▼
Agent.selectRelevantTools() ──→ 筛选相关工具 [save_version, view_status, update_user_info]
    │
    ▼
LangChain LLM.GenerateContent(tools=[...]) ──→ LLM 返回 ToolCall: save_version(message="...")
    │
    ▼
Agent.handleLangChainToolCalls() ──→ 执行 save_version 工具
    │
    ▼
工具结果回传 LLM ──→ LLM 生成最终回复 "✅ 已保存新版本 #abc1234"
```

### 4.1 LLM 接入层 (llm/langchain.go) —— 连接大模型

这是整个 LLM 模式的起点。用 langchaingo 创建 LLM 实例：

```go
// internal/llm/langchain.go

// OpenAIConfig OpenAI 兼容 API 配置
type OpenAIConfig struct {
    APIKey    string        `json:"api_key"`
    BaseURL   string        `json:"base_url"`    // 默认 https://api.openai.com/v1
    Model     string        `json:"model"`       // 默认 gpt-4o
    MaxTokens int           `json:"max_tokens"`  // 默认 4096
    Timeout   time.Duration `json:"timeout"`     // 默认 60s
}

// Usage token 使用量
type Usage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
    TotalTokens      int `json:"total_tokens"`
}

// NewLangChainLLM 基于 LangChain Go 框架创建 LLM 实例
// 支持所有 OpenAI 兼容的 API 提供商（OpenAI、Azure、DeepSeek、通义千问、Ollama 等）
func NewLangChainLLM(config OpenAIConfig) (llms.Model, error) {
    opts := []openai.Option{
        openai.WithToken(config.APIKey),
    }
    if config.Model != "" {
        opts = append(opts, openai.WithModel(config.Model))
    }
    if config.BaseURL != "" {
        opts = append(opts, openai.WithBaseURL(config.BaseURL))
    }
    // 创建 OpenAI 兼容的 LLM 实例
    llm, err := openai.New(opts...)
    if err != nil {
        return nil, fmt.Errorf("创建 LangChain LLM 失败: %w", err)
    }
    return llm, nil
}
```

> **为什么用 `openai.New()` 而不是别的？** 因为 OpenAI 的 Chat API 已经成为事实标准，几乎所有大模型厂商（DeepSeek、通义千问、Ollama 等）都兼容这个接口。你只需要换 `BaseURL` 和 `Model` 就能切换不同的模型。

### 4.2 工具定义 (llm/tools.go) —— 告诉 LLM 它能做什么

**这是 LLM Agent 最核心的概念：Function Calling（工具调用）。**

你需要把 Agent 能做的事情，用 JSON Schema 的格式告诉 LLM，LLM 就能根据用户输入选择合适的工具并填写参数。

```go
// internal/llm/tools.go

// GitAgentTool 工具定义结构
type GitAgentTool struct {
    Name        string `json:"name"`
    Description string `json:"description"`
    Parameters  any    `json:"parameters"` // JSON Schema
}

// AllGitAgentTools 所有 Git Agent 可用的工具定义（16 个）
var AllGitAgentTools = []GitAgentTool{
    {
        Name:        "save_version",
        Description: "保存当前文件修改为新版本。用户完成编辑后使用此功能保存。**重要规则**：直接调用本工具即可，系统会自动查看修改内容来生成 commit message。你只需提供 commit message 参数。",
        Parameters: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "message": map[string]any{
                    "type":        "string",
                    "description": "Commit message in English. Use conventional commit style...",
                },
                "files": map[string]any{
                    "type":        "string",
                    "description": "要保存的文件路径，多个用逗号分隔。留空表示保存所有修改",
                },
            },
            "required": []string{"message"},
        },
    },
    // ... 更多工具：view_history, restore_version, view_diff, view_status,
    //     submit_change, view_team_change, merge_branch, init_repo,
    //     create_branch, switch_branch, list_branches, create_tag,
    //     push_to_remote, pull_from_remote, detect_conflict,
    //     resolve_conflict, update_user_info
}
```

> **工具定义的 Description 非常重要！** LLM 完全依赖 Description 来决定什么时候该调用这个工具、参数该怎么填。写好 Description 是 Agent 质量的关键。好的 Description 应该：1）说清楚这个工具做什么；2）什么情况下该用；3）参数的填写要求和示例。

**工具设计 Checklist：**

- [ ] **Description 要极其详细**：LLM 完全依赖 Description 决定是否调用和怎么填参数
- [ ] **参数要有明确描述**：每个参数的 type、description、示例
- [ ] **required 精确标注**：不要把可选参数标为 required
- [ ] **同名工具要避免**：不同意图不要定义同名工具
- [ ] **参数类型尽量简单**：string 为主，避免嵌套对象（LLM 填复杂参数容易出错）

### 4.3 工具注册中心 (llm/git_tools.go) —— 把定义和执行绑在一起

**工具定义只是"告诉 LLM 有什么"，而工具注册中心把"定义"和"执行器"绑定起来。**

```go
// internal/llm/git_tools.go

// GitTool 将 GitAgentTool 适配为 langchaingo tools.Tool 接口
type GitTool struct {
    toolDef   GitAgentTool
    executors map[string]func(ctx context.Context, params map[string]interface{}) (string, error)
}

// NewGitTool 创建 LangChain Git 工具适配器
func NewGitTool(toolDef GitAgentTool, executor func(...) (string, error)) tools.Tool {
    // 适配器模式：把 GitAgentTool + executor → langchaingo tools.Tool
}

// GitToolRegistry 工具注册中心
type GitToolRegistry struct {
    tools     map[string]tools.Tool
    executors map[string]func(ctx context.Context, params map[string]interface{}) (string, error)
}

// Register 注册工具执行器（定义在 tools.go，实现在 agent.go）
func (r *GitToolRegistry) Register(name string, executor func(...) (string, error)) {
    r.executors[name] = executor
}

// BuildToolDefinitions 构建 LangChain llms.Tool 列表（用于 function calling）
func (r *GitToolRegistry) BuildToolDefinitions() []llms.Tool {
    var result []llms.Tool
    for _, toolDef := range AllGitAgentTools {
        if executor, ok := r.executors[toolDef.Name]; ok {
            // 同时创建 GitTool 实例并存入 tools map，确保 GetTool 能找到
            r.tools[toolDef.Name] = NewGitTool(toolDef, executor)
            result = append(result, llms.Tool{
                Type: "function",
                Function: &llms.FunctionDefinition{
                    Name:        toolDef.Name,
                    Description: toolDef.Description,
                    Parameters:  toolDef.Parameters,
                },
            })
        }
    }
    return result
}

// GetTool 获取指定名称的工具（执行器）
func (r *GitToolRegistry) GetTool(name string) (tools.Tool, bool) {
    t, ok := r.tools[name]
    return t, ok
}
```

> **为什么要分 tools.go 和 git_tools.go？** tools.go 只负责声明"有哪些工具"（纯数据），git_tools.go 负责把定义适配成 LangChain 框架的接口。这种分离让你可以在不同项目中复用适配逻辑，只替换工具定义即可。

### 4.4 系统提示词 (agent.go buildSystemPrompt) —— 告诉 LLM 它是谁

**注意：git-agent 没有单独的 `prompts.go` 文件，系统提示词构建逻辑在 `agent.go` 的 `buildSystemPrompt()` 方法中。**

```go
// agent.go 中的 buildSystemPrompt()

func (a *Agent) buildSystemPrompt(intent string) string {
    var sb strings.Builder

    // 1. 角色骨架（始终包含）
    sb.WriteString(`你是一个文件版本管理助手（Git Agent），专门帮助不懂技术的办公人员管理文件版本。

## 你的角色
- 你是用户的文件管家，帮助用户保存、查看、恢复文件版本
- 你用简单易懂的办公语言与用户交流，绝不使用 git 术语
- 你将用户的自然语言需求转化为具体的版本管理操作

## 注意事项
- 操作前先确认用户意图，如果模糊可以追问
- 涉及不可逆操作时，先提醒用户确认
- 绝对不要向用户暴露任何 git 命令或技术术语

## 输出要求
当你需要执行操作时，请调用对应的工具函数。系统会自动执行并将结果返回给你。
然后你需要把执行结果用用户友好的语言转述给用户。
`)

    // 2. 动态注入 Skills/Rules（按意图按需加载）
    if a.promptKit != nil {
        dynamicPrompt := a.promptKit.GetPromptForIntents([]string{intent})
        sb.WriteString(dynamicPrompt)
    }

    return sb.String()
}
```

> **为什么要分"骨架 + 动态注入"？** 因为如果每次都把所有规则都塞进系统提示词，token 浪费严重，而且 LLM 容易被太多规则干扰。按意图只加载相关规则，既省 token 又更精准。

### 4.5 Agent 的 LLM 模式 —— ReAct 循环

**这是 LLM Agent 的核心模式：Reasoning + Acting 循环。**

```
用户输入 → LLM 推理 → 调用工具 → 执行工具 → 结果回传 LLM → LLM 继续推理 → ... → 生成最终回复
```

在代码中的实现：

```go
// processWithLLM —— LLM 模式的入口
func (a *Agent) processWithLLM(ctx context.Context, input string) *AgentResponse {
    a.setState(StateThinking)

    // 1. 按需注入意图相关的 Skills/Rules
    //    在用户输入前插入一条系统提示，告知 LLM 当前操作应遵循的规则
    if a.promptKit != nil {
        intent := a.detectIntentType(input)
        intentPrompt := a.promptKit.GetPromptForIntents([]string{intent})
        if intentPrompt != "" {
            a.chatHistory = append(a.chatHistory,
                llms.TextParts(llms.ChatMessageTypeSystem,
                    "[当前操作的适用规则]\n"+intentPrompt),
            )
        }
    }

    // 2. 将用户输入加入对话上下文
    a.chatHistory = append(a.chatHistory,
        llms.TextParts(llms.ChatMessageTypeHuman, input),
    )

    // 3. 智能工具筛选：先用本地 interpreter 预判意图，只发送相关工具给 LLM
    //    这对于小模型（如 7B）特别重要，工具太多会导致模型不调用任何工具
    a.currentTools = a.selectRelevantTools(input)

    // 4. 调用 LLM，携带筛选后的工具定义（function calling）
    resp, err := a.langchainLLM.GenerateContent(
        ctx,
        a.chatHistory,
        llms.WithTools(a.currentTools),
        llms.WithToolChoice("auto"),            // 显式指定 auto，确保 Ollama 等兼容 API 启用工具调用
        llms.WithTemperature(0.1),              // Agent 场景使用低温度，减少幻觉
        llms.WithMaxTokens(a.llmConfig.MaxTokens),
        openai.WithLegacyMaxTokensField(),      // 兼容 Ollama、DeepSeek 等非 OpenAI API
    )
    if err != nil {
        // LLM 调用失败，回退到本地模式
        a.chatHistory = a.chatHistory[:len(a.chatHistory)-1]
        return a.fallbackToLocal(ctx, input, err)
    }

    // 5. 处理 LLM 响应，从第 1 次迭代开始
    return a.handleLLMResponse(ctx, resp, 1)
}

// handleLLMResponse —— 处理 LLM 响应的入口（委托给 handleLLMResponseWithUsage）
func (a *Agent) handleLLMResponse(ctx context.Context, resp *llms.ContentResponse, iteration int) *AgentResponse {
    return a.handleLLMResponseWithUsage(ctx, resp, iteration, nil)
}

// handleLLMResponseWithUsage —— 处理 LLM 响应（含 token 用量累计）
func (a *Agent) handleLLMResponseWithUsage(ctx context.Context, resp *llms.ContentResponse,
    iteration int, accumUsage *llm.Usage, callHistory ...map[string]int) *AgentResponse {

    choice := resp.Choices[0]
    // 从 GenerationInfo 中提取 Token 用量，跨轮次累计
    var totalUsage llm.Usage
    if accumUsage != nil { totalUsage = *accumUsage }
    // ... 从 choice.GenerationInfo 提取 PromptTokens/CompletionTokens/TotalTokens ...

    assistantContent := choice.Content
    toolCalls := choice.ToolCalls

    // 将 assistant 消息加入对话历史（含文本和工具调用）
    aiParts := []llms.ContentPart{}
    if assistantContent != "" {
        aiParts = append(aiParts, llms.TextContent{Text: assistantContent})
    }
    for _, tc := range toolCalls {
        aiParts = append(aiParts, tc)
    }
    a.chatHistory = append(a.chatHistory, llms.MessageContent{
        Role: llms.ChatMessageTypeAI, Parts: aiParts,
    })

    if len(toolCalls) > 0 {
        // LLM 要求调用工具 → 执行工具 → 结果回传 LLM → 递归处理
        return a.handleLangChainToolCalls(ctx, toolCalls, &totalUsage, iteration, ...)
    }

    // LLM 直接回复文本（没有工具调用）
    return &AgentResponse{Success: true, Message: assistantContent, TokenUsage: &totalUsage}
}

// handleLangChainToolCalls —— 执行工具调用（核心防循环逻辑）
func (a *Agent) handleLangChainToolCalls(ctx context.Context, toolCalls []llms.ToolCall,
    totalUsage *llm.Usage, iteration int, toolCallHistory ...map[string]int) *AgentResponse {

    // 合并跨轮次的工具调用历史（用于检测重复调用）
    callHistory := make(map[string]int)
    // ... 合并 toolCallHistory ...

    // 终止性工具：save_version/submit_change/push_to_remote/restore_version
    // 成功执行后，操作已完成，LLM 不需要再调用查看类工具
    terminalToolCalled := false
    terminalTools := map[string]bool{
        "save_version": true, "submit_change": true,
        "push_to_remote": true, "restore_version": true,
    }

    for _, tc := range toolCalls {
        toolName := tc.FunctionCall.Name
        toolArgs := tc.FunctionCall.Arguments

        // 跨轮次重复调用检测：
        //   查看类工具（view_*）2 次即终止，操作类工具 3 次终止
        callHistory[toolName]++
        maxCalls := 3
        if strings.HasPrefix(toolName, "view_") { maxCalls = 2 }
        if callHistory[toolName] > maxCalls {
            return &AgentResponse{Success: false, Message: "工具被重复调用过多，操作可能已完成"}
        }

        // 通过注册中心执行工具
        tool, ok := a.toolRegistry.GetTool(toolName)
        if !ok {
            // 工具不存在，将错误信息回传 LLM 让它自行修正
            // ... 加入 chatHistory ...
            continue
        }
        result, err := tool.Call(ctx, toolArgs)
        // ... 错误处理，AuthError 友好提示 ...

        if terminalTools[toolName] && err == nil {
            terminalToolCalled = true
        }

        // 将工具结果加入对话历史
        //   对重复调用的查看类工具，附加 SYSTEM NOTICE 引导 LLM 生成最终回复
        a.chatHistory = append(a.chatHistory, llms.MessageContent{
            Role: llms.ChatMessageTypeTool,
            Parts: []llms.ContentPart{
                llms.ToolCallResponse{ToolCallID: tc.ID, Name: toolName, Content: result},
            },
        })
    }

    // 将工具执行结果回传 LLM
    // 关键优化：终止性工具成功后，过滤掉查看类工具，防止 LLM 反复确认
    toolsForLLM := a.currentTools
    if terminalToolCalled {
        var filteredTools []llms.Tool
        for _, tool := range a.currentTools {
            name := tool.Function.Name
            if !strings.HasPrefix(name, "view_") && name != "detect_conflict" {
                filteredTools = append(filteredTools, tool)
            }
        }
        toolsForLLM = filteredTools
        if len(toolsForLLM) == 0 { toolsForLLM = nil }
    }

    resp, _ := a.langchainLLM.GenerateContent(ctx, a.chatHistory, ...)

    // 递归处理（LLM 可能继续调用工具 = 多轮 ReAct）
    nextIteration := iteration + 1
    if nextIteration > maxReActIterations {
        return &AgentResponse{Success: false, Message: "操作陷入循环，已自动终止"}
    }
    return a.handleLLMResponseWithUsage(ctx, resp, nextIteration, totalUsage, callHistory)
}
```

**ReAct 循环的关键机制：**

1. **递归处理**：LLM 可能一次调用不够，需要多轮（比如先查看状态 → 再保存版本）
2. **迭代上限**：`maxReActIterations = 5`，防止 LLM 陷入无限循环
3. **跨轮次重复调用检测**：查看类工具（`view_*`）2 次即终止，操作类工具 3 次终止；同时附加 SYSTEM NOTICE 引导 LLM 生成最终回复
4. **终止性工具过滤**：`save_version`/`submit_change`/`push_to_remote`/`restore_version` 成功后，从工具列表中移除查看类工具，防止 LLM 反复确认
5. **Token 用量累计**：`handleLLMResponseWithUsage` 跨轮次累计 token 使用量，最终返回给用户
6. **LLM 失败回退**：`processWithLLM` 中 LLM 调用失败时，自动回退到 `fallbackToLocal` 本地模式
7. **对话历史管理**：每一轮的 assistant 消息（含文本和工具调用）都加入 chatHistory，LLM 根据完整上下文做决策

---

## 第五步：动态提示词系统 (promptkit) —— 让 Agent 可配置、可热更新

**为什么需要 promptkit？** 早期所有规则都硬编码在系统提示词里，有三个问题：
1. 每次改规则都要改代码、重新编译
2. 不同用户/项目可能需要不同的规则
3. 所有规则全量注入，浪费 token 且干扰 LLM

promptkit 的设计思路：

```
promptkit/resources/
├── skills/                         # 技能（操作知识）
│   ├── batch-commit.md             # 分批提交技能
│   ├── conflict-resolution.md      # 冲突解决技能
│   └── push-fail-guide.md          # 推送失败引导技能
└── rules/                          # 规则（行为约束）
    ├── always-execute.md           # 直接执行规则
    ├── commit-message.md           # Commit message 规范
    ├── display-format.md           # 显示格式规则
    ├── no-git-terms.md             # 术语翻译规则
    ├── no-repeat-tools.md          # 不重复调用工具规则
    └── version-restore.md          # 版本恢复确认规则
```

**规则文件的元数据格式**（通过 HTML 注释嵌入）：

```markdown
<!-- meta: {"intents":["save_version","submit_change"],"priority":10,"description":"Commit message 撰写规范"} -->

# Commit Message 撰写规范

当执行 save_version 或 submit_change 操作时，message 参数必须遵循以下规则：
1. 必须使用英文
2. 使用 conventional commit 风格
...
```

- `intents`：这条规则关联哪些意图，只有匹配到这些意图时才注入
- `priority`：排序优先级，数字越小越先出现
- 空 `intents`：表示全局生效，始终注入（如 `no-git-terms.md`）

**三层配置 + 覆盖机制：**

```
内置 (embed.FS) → 用户级 (~/.config/git-agent/) → 项目级 (.git-agent/)
```

同名文件后者覆盖前者，让用户可以自定义或覆盖内置规则。

**核心 API：**

```go
// 创建 Kit 实例
kit := promptkit.NewKit(promptkit.ResourcesFS, promptkit.Config{
    UserDir:    "~/.config/git-agent",   // 用户级
    ProjectDir: ".git-agent",            // 项目级
})

// 加载所有 Skills/Rules
kit.Load()

// 根据意图获取需要注入的提示词（自动包含全局规则 + 意图关联规则，按 Priority 排序）
prompt := kit.GetPromptForIntents([]string{"save_version"})

// 启动热加载（监听文件变更，300ms 防抖后自动重载）
kit.WatchAndHotReload()

// 关闭
kit.Close()
```

**热加载机制：** 使用 `fsnotify` 监听用户级和项目级目录的 `.md` 文件变更，300ms 防抖后自动全量重载。运行期间新建的自定义目录也会被动态补充监听。

---

## 第六步：完整数据流追踪 —— 一条用户消息从头到尾经历了什么

以用户输入 **"保存修改"** 为例，追踪完整链路：

### LLM 模式

```
1. main.go: readline 读取用户输入 "保存修改"

2. agent.Process(ctx, "保存修改")
   ├── IsLLMEnabled() == true → 走 processWithLLM()

3. processWithLLM():
   ├── selectRelevantTools("保存修改")
   │   ├── interpreter.Parse("保存修改") → IntentSaveVersion
   │   └── intentToolMapping["save_version"] → [save_version, view_status, update_user_info]
   │   └── 返回这3个工具的 llms.Tool 定义
   │
   ├── promptKit.GetPromptForIntents(["save_version"])
   │   └── 返回: always-execute 规则 + commit-message 规则 + batch-commit 技能
   │
   ├── chatHistory 追加: SystemMessage[规则] + HumanMessage["保存修改"]
   │
   └── langchainLLM.GenerateContent(
         chatHistory,
         WithTools=[save_version, view_status, update_user_info],
         WithToolChoice="auto",
         WithTemperature=0.1,
       )
       │
       ▼ LLM 推理后返回 ToolCall
       resp.Choices[0].ToolCalls = [{Name: "save_version", Arguments: '{"message":"feat: save changes"}'}]

4. handleLLMResponse() → handleLangChainToolCalls()
   ├── toolRegistry.GetTool("save_version") → 找到执行器
   ├── 执行器内部:
   │   ├── isVagueCommitMessage("feat: save changes") → true（太笼统！）
   │   ├── 自动获取 diff 摘要
   │   └── 返回错误: "commit message 'feat: save changes' is too vague..."
   │
   ├── chatHistory 追加: ToolMessage[{result: "too vague..."}]
   │
   └── langchainLLM.GenerateContent(chatHistory, tools=...) → 第2轮推理
       │
       ▼ LLM 根据 diff 摘要重写 commit message
       resp.Choices[0].ToolCalls = [{Name: "save_version", Arguments: '{"message":"feat: add user login feature"}'}]

5. handleLangChainToolCalls()（第2轮）
   ├── 执行 save_version("feat: add user login feature")
   ├── gitWrapper.SaveVersion() → git add + git commit → 成功！
   ├── terminalToolCalled = true → 过滤掉查看类工具
   │
   ├── chatHistory 追加: ToolMessage[{result: "saved successfully"}]
   │
   └── langchainLLM.GenerateContent(chatHistory, tools=nil)
       │
       ▼ LLM 生成最终回复（纯文本，不再调用工具）
       resp.Choices[0].Content = "✅ 已保存新版本 #abc1234"

6. handleLLMResponse() → 返回 AgentResponse
   └── {Success: true, Message: "✅ 已保存新版本 #abc1234", UsedLLM: true, TokenUsage: {...}}

7. main.go: printResponse() → 用户看到友好提示
```

### 本地模式（降级路径）

```
1. main.go: readline 读取用户输入 "保存修改"

2. agent.Process(ctx, "保存修改")
   └── IsLLMEnabled() == false → 走 processLocal()

3. processLocal():
   ├── interpreter.Parse("保存修改") → UserIntent{Type: IntentSaveVersion, Confidence: 1.0}
   │
   ├── planner.CreatePlan(IntentSaveVersion) → Plan{Steps: [{git_add}, {git_commit}]}
   │
   └── executePlan():
       ├── executeStep(git_add) → gitWrapper.AddAll() → 成功
       └── executeStep(git_commit) → gitWrapper.Commit("保存修改", ...) → 成功

4. formatResponse() → interpreter.TranslateResult() → "✅ 已保存新版本 #abc1234"

5. main.go: printResponse() → 用户看到友好提示
```

---

## 通用 Agent 开发模板 —— AI + Anything 复用清单

基于 git-agent 的架构，提炼出一套可复用的开发模板。你只需要替换业务逻辑部分，就能快速构建一个新的 AI Agent 应用。

### 需要替换的部分

| git-agent 中的模块 | 通用角色 | 你需要做的 |
|---|---|---|
| `gitwrapper` | **业务操作层** | 封装你的业务操作为 Go 函数（数据库操作、API 调用、文件操作等） |
| `interpreter.keywords` | **意图关键词** | 列出你的领域中的意图和关键词 |
| `planner.CreatePlan` | **意图→步骤映射** | 定义每个意图需要执行哪些步骤 |
| `agent.executeStep` | **步骤→操作映射** | 把步骤类型映射到业务操作函数 |
| `llm/tools.go AllGitAgentTools` | **LLM 工具定义** | 用 JSON Schema 定义你的工具（名字、描述、参数） |
| `agent.registerToolExecutors` | **工具执行器** | 把每个工具名映射到具体的业务操作函数 |
| `agent.buildSystemPrompt` | **系统提示词** | 定义 Agent 的角色和基本行为规范 |
| `promptkit/resources/` | **Skills/Rules** | 编写你的领域的技能和规则 Markdown 文件 |

### 不需要改的部分

| 模块 | 为什么不用改 |
|---|---|
| `agent/agent.go` 核心循环 | 感知→推理→行动→反馈的循环是通用的 |
| `agent/agent.go` ReAct 循环 | LLM 调用工具→结果回传→继续推理的模式是通用的 |
| `agent/agent.go` 状态管理 | idle/thinking/planning/executing/error 状态机是通用的 |
| `interpreter/interpreter.go` 匹配算法 | 多策略匹配 + 评分 + 消歧的算法是通用的 |
| `planner/planner.go` 框架 | Plan/Step/ExecutionResult 数据结构是通用的 |
| `llm/langchain.go` | LLM 创建逻辑是通用的 |
| `llm/git_tools.go` 注册中心 | 工具定义↔执行器的映射机制是通用的 |
| `promptkit/` 全部 | Skills/Rules 加载、意图索引、热更新机制是通用的 |
| `conflict/` | 冲突检测模式可复用（你的领域也可能有冲突） |
| `repository/` | 仓库管理模式可复用 |
| `main.go` 交互循环 | readline 输入→Agent.Process→输出响应的模式是通用的 |

### 开发顺序建议

```
Phase 1: 先跑通本地模式（约 2-3 天）
  ├── Day 1: 业务操作层 + 意图定义
  ├── Day 2: interpreter + planner + agent 核心循环
  └── Day 3: main.go 交互循环 + 调试

Phase 2: 引入 LLM（约 2-3 天）
  ├── Day 4: llm/langchain.go + 工具定义 + 注册执行器
  ├── Day 5: ReAct 循环 + 对话历史管理
  └── Day 6: 调试 LLM 工具调用、优化提示词

Phase 3: 动态提示词 + 生产化（约 2-3 天）
  ├── Day 7: promptkit Skills/Rules 编写 + 热加载
  ├── Day 8: 错误处理降级 + 认证 + 安全
  └── Day 9: 端到端测试 + 性能优化
```

---

## 踩坑经验与注意事项

### 1. LLM 工具定义的 Description 要写得非常具体

LLM 完全依赖 Description 来决定是否调用工具和怎么填参数。模糊的 Description 会导致：
- LLM 在不该调用时调用了
- 参数填写错误
- 多个相似工具之间选错

**反面案例：**
```
Description: "保存文件"
```

**正面案例：**
```
Description: "保存当前文件修改为新版本。用户完成编辑后使用此功能保存。**重要规则**：直接调用本工具即可，系统会自动查看修改内容来生成 commit message。你只需提供 commit message 参数。"
```

### 2. 工具筛选对小模型至关重要

小模型（7B-14B）面对过多工具时，容易出现"不调用任何工具"或"调用错误工具"的问题。git-agent 的解决方案：

```go
// 先用本地 interpreter 预判意图，只发送相关工具
var intentToolMapping = map[string][]string{
    "save_version": {"save_version", "view_status", "update_user_info"},
    "view_history": {"view_history", "view_status"},
    // ...
}
```

### 3. ReAct 循环需要防无限循环

LLM 有时会反复调用同一个查看类工具而不采取行动。需要：
- 迭代上限（5次）
- 跨轮次工具调用计数（同一工具调用2-3次就终止）
- 终止性工具标记（save_version 成功后移除查看类工具）

```go
const maxReActIterations = 5

// 检测跨轮次重复调用
if callHistory[toolName] > maxCallsForTool {
    return errorResponse("工具 %s 被重复调用多次")
}
```

### 4. 对话历史会越来越长，需要管理

每次 ReAct 循环都会往 chatHistory 中添加多条消息（用户消息 + AI消息 + 工具调用 + 工具结果），几轮对话后 token 数可能超限。建议：
- 提供 `/clear` 命令清空历史
- 设置对话历史长度上限
- 定期截断最早的对话轮次

### 5. 本地模式是不可或缺的保底

LLM 可能因为各种原因不可用：API 超时、额度用完、模型升级导致行为变化…本地模式保证 Agent 始终可用。降级策略：

```go
func (a *Agent) processWithLLM(ctx context.Context, input string) *AgentResponse {
    resp, err := a.langchainLLM.GenerateContent(...)
    if err != nil {
        // LLM 失败，自动降级到本地模式
        return a.fallbackToLocal(ctx, input, err)
    }
    // ...
}
```

### 6. 工具执行结果要给 LLM 明确信号

LLM 有时不知道操作是否真的完成了，会反复调用查看类工具确认。在工具结果中附加明确信号：

```go
// 成功保存后
return jsonResult + "\n[Version saved successfully. Do NOT call view_diff or view_status to verify.]"

// 没有修改时
return "没有发现修改。当前工作区与已保存版本完全一致。请直接基于此信息回复用户，不要再次调用 view_diff。"
```

### 7. commit message 质量校验是必要的

LLM 生成的 commit message 经常过于笼统（如 "update files"、"save changes"），需要在执行器中校验并要求重写：

```go
if isVagueCommitMessage(message) {
    // 自动获取 diff 摘要，帮助 LLM 重写
    diffHint := a.gitWrapper.Diff("")
    return "", fmt.Errorf("commit message '%s' is too vague. Here is a summary of changes:\n%s\nPlease rewrite.", message, diffHint)
}
```

### 8. Temperature 设 0.1，不要设 0

Agent 场景需要确定性输出，但 Temperature=0 在某些 API（如 Ollama）上会导致问题。0.1 是一个安全的折中值。

### 9. `openai.WithLegacyMaxTokensField()` 兼容性选项

Ollama、DeepSeek 等非 OpenAI API 对 `max_tokens` 字段的处理不同，需要加这个选项确保兼容：

```go
resp, err := a.langchainLLM.GenerateContent(
    ctx, a.chatHistory,
    llms.WithMaxTokens(a.llmConfig.MaxTokens),
    openai.WithLegacyMaxTokensField(),  // 兼容非 OpenAI API
)
```

### 10. 表格等格式化内容要硬编码，不要交给 LLM

LLM 输出表格经常丢列、截断内容。git-agent 的做法是在工具执行器中硬编码格式化为 Markdown 表格，并在结果中附加 SYSTEM NOTICE 要求 LLM 原样输出：

```go
sb.WriteString("\n\n[SYSTEM] 以上表格已经是格式化好的最终展示内容，你必须遵守以下规则：")
sb.WriteString("\n1. 必须原样展示完整表格，不得删减列、不得改写表格内容。")
```

---

> **总结**：开发一个 Agent 应用的核心链路是：**业务操作封装 → 意图解析 → 执行规划 → Agent 循环 → LLM 接入 → 动态提示词**。先把本地模式跑通，再加 LLM 增强，最后做动态化和生产化。这套模式可以复用到任何 AI + Anything 的场景。

---

## 调试和测试方法

### 单元测试

git-agent 中纯本地组件（Interpreter、Planner）不依赖 LLM，可以直接测试：

```go
// internal/interpreter/interpreter_test.go
func TestInterpreter_Parse_SaveVersion(t *testing.T) {
    interp := interpreter.New()
    
    intent, err := interp.Parse("保存修改")
    assert.NoError(t, err)
    assert.Equal(t, interpreter.IntentSaveVersion, intent.Type)
    assert.GreaterOrEqual(t, intent.Confidence, 0.5)
}

// internal/planner/planner_test.go
func TestPlanner_CreatePlan_SaveVersion(t *testing.T) {
    p := planner.New()
    intent := &interpreter.UserIntent{Type: interpreter.IntentSaveVersion}
    
    plan, err := p.CreatePlan(intent)
    assert.NoError(t, err)
    assert.Equal(t, 2, plan.TotalSteps)  // add + commit
}
```

### 集成测试

涉及 LLM 的测试需要真实的 API Key，建议用 `testing.Short()` 区分：

```go
func TestAgent_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("跳过集成测试")
    }
    
    llmConfig := &agent.LLMConfig{
        Enabled: true,
        APIKey:  os.Getenv("GIT_AGENT_API_KEY"),
        Model:   os.Getenv("GIT_AGENT_MODEL"),
    }
    
    a, err := agent.NewWithLLM(testRepoPath, testUserConfig, llmConfig)
    assert.NoError(t, err)
    
    resp := a.Process(context.Background(), "保存修改")
    assert.True(t, resp.Success)
    assert.True(t, resp.UsedLLM)
}
```

### 调试技巧

1. **双模式对比调试**：同一输入分别在本地模式和 LLM 模式运行，对比结果差异

2. **对话历史检查**：打印 chatHistory 的每条消息，查看 LLM 的工具调用链

3. **工具调用追踪**：在 GitTool.Call() 中添加日志，记录每次工具调用的名称、参数和结果

4. **Prompt 调试**：打印完整的系统提示词（骨架 + 动态注入），检查 Skills/Rules 是否正确加载

---

## 进阶功能扩展

### 对话历史管理优化

当前 git-agent 的对话历史会无限增长。待优化方向：

- 设置对话历史长度上限（当前会无限增长）
- 定期截断最早的对话轮次
- 对超长的工具结果进行摘要

### 多模态支持

```go
// langchaingo 已支持图片输入
messages = append(messages, llms.MessageContent{
    Role: llms.ChatMessageTypeHuman,
    Parts: []llms.ContentPart{
        llms.TextPart("这张截图显示了什么错误？"),
        llms.ImagePart(imageData),
    },
})
```

### 插件系统

当前 git-agent 通过 `llm/git_tools.go` 的注册中心实现工具扩展。如果需要更灵活的插件机制：

```go
// 插件接口
type Plugin interface {
    Name() string
    ToolDefinition() llm.GitAgentTool
    Execute(ctx context.Context, params map[string]interface{}) (string, error)
}

// 插件管理器
type PluginManager struct {
    plugins map[string]Plugin
}

func (pm *PluginManager) Register(plugin Plugin) {
    pm.plugins[plugin.Name()] = plugin
    // 同时注册到 GitToolRegistry
    registry.Register(plugin.Name(), plugin.Execute)
}
```

### 流式响应

```go
// langchaingo 支持流式输出
stream, err := a.langchainLLM.GenerateContentStream(ctx, messages, options...)
for chunk := range stream {
    fmt.Print(chunk.Choices[0].Content)  // 实时输出
}
```

---

## 参考资源

- [Langchaingo GitHub](https://github.com/tmc/langchaingo)
- [LangChain 官方文档](https://python.langchain.com/)
- [ReAct Paper](https://arxiv.org/abs/2210.03629)
- [Agent 设计模式（Anthropic）](https://www.anthropic.com/research/building-effective-agents)
