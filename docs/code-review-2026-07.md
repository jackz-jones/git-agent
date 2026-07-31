# Code Review 修复与优化总结（2026-07）

> 本文档记录了 2026 年 7 月针对 `git-agent` 项目的一次系统性 code review 及其后续的
> 全量修复与优化工作。共覆盖 **21 项需求**，分为 **11 个任务组** 落地实施，涉及
> Bug 修复、安全加固、健壮性提升、可维护性改进以及关键路径单元测试补齐。
>
> - 计划文档：[`.codebuddy/plan/code-review-fixes/requirements.md`](../.codebuddy/plan/code-review-fixes/requirements.md)
> - 任务清单：[`.codebuddy/plan/code-review-fixes/task-item.md`](../.codebuddy/plan/code-review-fixes/task-item.md)
> - 修复完成日期：2026-07-31

---

## 目录

- [一、Review 结论与总体分布](#一review-结论与总体分布)
- [二、变更文件总览](#二变更文件总览)
- [三、按需求逐项落地](#三按需求逐项落地)
  - [🔴 高优 — Bug / 安全](#-高优--bug--安全)
  - [🟡 中优 — 健壮性 / 数据正确性](#-中优--健壮性--数据正确性)
  - [🟢 低优 — 可维护性 / UX](#-低优--可维护性--ux)
- [四、单元测试补齐](#四单元测试补齐)
- [五、验收与回归](#五验收与回归)
- [六、遗留事项与后续建议](#六遗留事项与后续建议)

---

## 一、Review 结论与总体分布

本次 review 覆盖的核心模块：

| 模块 | 主要文件 | 关键关注点 |
|------|---------|-----------|
| CLI 入口 | `main.go` | 交互模式、`/mode` 切换 |
| Agent 核心 | `internal/agent/agent.go` | ReAct 循环、工具注册、LLM/Local 模式 |
| Git 封装 | `internal/gitwrapper/gitwrapper.go` | `SaveVersion` / `Diff` / `Merge` / `Status` / `Push` |
| 冲突处理 | `internal/conflict/conflict.go` | 冲突检测与解决 |
| 意图解析 | `internal/interpreter/interpreter.go` | 本地意图解析 |
| LLM | `internal/llm/` | 工具定义与注册中心 |
| Web 层 | `internal/web/` | SSE / Workspace / Handlers / 鉴权 |
| Prompt 装载 | `internal/promptkit/` | Skills/Rules 动态加载 |

按严重度分布：

- 🔴 **高（6 项）**：Bug / 安全类
- 🟡 **中（7 项）**：健壮性 / 数据正确性
- 🟢 **低（8 项）**：可维护性 / UX

---

## 二、变更文件总览

| 分类 | 文件 | 变更类型 |
|------|------|---------|
| Git 封装 | `internal/gitwrapper/gitwrapper.go` | 修改（Bug 修复 + 加固） |
| Git 封装 | `internal/gitwrapper/gitwrapper_test.go` | **新增**（10 个测试） |
| Agent | `internal/agent/agent.go` | 修改（生命周期、上下文治理、UX） |
| Agent | `internal/agent/redact.go` | **新增**（敏感字段脱敏） |
| Web | `internal/web/auth.go` | **新增**（LAN Token 鉴权） |
| Web | `internal/web/handlers_browse.go` | 修改（`.git` / reservedDirs 屏蔽） |
| Web | `internal/web/handlers.go`（SSE） | 修改（断连处理 + recover） |
| Web | `internal/web/workspace_manager.go` | 修改（`ReloadLLMConfig` 加锁 + 上下文迁移） |
| Web | `internal/web/serve.go` | 修改（一次性 Token 生成） |
| CLI | `main.go` | 修改（`/mode` 真正切换） |

---

## 三、按需求逐项落地

### 🔴 高优 — Bug / 安全

#### 需求 1 & 17：`Diff()` / `Status()` 暂存状态处理 Bug

**问题**：
- `Diff()` 中若文件"仅暂存修改/删除"，无 patch 输出，只显示状态。
- `Status()` 复用同一个 `FileChange` 实例往 `Staged` 和 `Unstaged` 里 append，
  两个字段实际指向同一份数据，混合状态下语义错乱。

**修复**：
- `Diff()` 补齐 `staging == git.Deleted` 的分支，输出对应 patch。
- `Status()` 拆为两个独立 `FileChange` 实例，`Staged` 与 `Unstaged`
  各自持有独立视图。

#### 需求 3：`Merge()` 冲突检测 / 作者硬编码 / patch 应用错误

**问题**：
- 冲突检测只识别 `edit_edit`，遗漏 `edit_delete` / `add_add`。
- 无冲突路径用 patch chunk 拼接，丢失本地独有行。
- author 硬编码为 `Git Agent`，无法从 `.git/config` 读取真实作者。

**修复**：
- `detectMergeConflicts` 覆盖三类冲突：`edit_edit / edit_delete / add_add`。
- 无冲突路径改为按 blob 直写，不再拼接损坏的 patch chunk。
- 作者信息从 `GetLocalUserConfig()` 获取；缺失时返回明确错误
  `用户信息未配置：请先设置 user.name 与 user.email 后再执行合并`。

#### 需求 8 / 18 / 11 / 12：Web 侧安全加固

**问题**：
- `/api/browse` 允许任意目录浏览（含 `.git`）。
- LAN 模式无鉴权，任何同网段客户端都能访问。
- SSE handler 在客户端断开后 Agent 继续运行，
  `flusher.Flush()` 写 broken pipe 无收敛。
- `handleTree` 用遍历下标截断，隐藏目录多时会过早截断。

**修复**：
- 新增 [`internal/web/auth.go`](../internal/web/auth.go)：
  - 生成一次性 Token（启动日志打印，只显示一次）。
  - 中间件校验 `Authorization: Bearer <token>` 或 `?token=` query。
  - 校验 `Origin` / `Referer` / `Host`，防跨站访问。
- `handlers_browse.go` 显式拒绝 `.git` 与 `reservedDirs`。
- `handleTree` 改为先过滤后截断，`maxEntries` 语义正确。
- `handleFile` 屏蔽 `.git` 目录下所有文件读取。
- SSE handler：
  - 引入 `stopped` 标志 + `sync.Once`，客户端断开后立即停止推送。
  - `sendEvent` 内部对 `Write` 失败调用 `recover`，收敛 broken pipe。
  - 客户端断开后主动中断 Agent 循环，避免"僵尸任务"占用资源。

#### 需求 13：`push_to_remote` 凭据脱敏与 URL 校验

**问题**：
- `remote_url` 直接来自 LLM 参数，可被 prompt injection 篡改。
- `emitToolCall` 把工具参数明文广播到 SSE，URL 中的密码会被前端和审计看到。

**修复**：
- `push_to_remote` 加入 `validateRemoteHTTPSURL`：
  - 仅允许 `https://` 协议（可扩展白名单）。
  - 校验 host 合法性、拒绝 `file://` / `git://` / `ssh://` 等危险协议。
  - 失败时回滚，不修改本地 remote 配置。
- 新增 [`internal/agent/redact.go`](../internal/agent/redact.go)：
  - `redactURL()`：对 URL 中的 userinfo 段 `user:pass@` 脱敏为 `user:***@`。
  - `redactToolArgs()`：对已知敏感字段（`remote_url`、`token`、`password` 等）
    统一脱敏。
  - `emitToolCall` 广播前、`chatHistory` 落库前均调用脱敏。

#### 需求 9 / 10 / 20：Agent 生命周期与并发安全

**问题**：
- `Agent.Close()` 非幂等，二次调用会 panic（channel 重复 close）。
- `ReloadLLMConfig` 直接换 Agent 实例，会丢弃 `chatHistory` 与 SSE hook。
- `Manager.ReloadLLMConfig` 有读锁 → 写锁竞态，多 workspace 并发触发会 panic。

**修复**：
- `Agent.Close()` 用 `sync.Once` 保证幂等。
- 新增：
  - `SnapshotChatHistory()` / `LoadChatHistory()`
  - `TransferEventHook()` / `EmitStateChanged()`
- `Manager.ReloadLLMConfig`：
  - 全程加写锁。
  - 迁移 `chatHistory`、SSE event hook 到新 Agent。
  - 异步回收旧 Agent，避免阻塞。

---

### 🟡 中优 — 健壮性 / 数据正确性

#### 需求 2：`isAncestor` 递归改迭代

**问题**：原实现递归无 `visited` 集合，diamond commit 图会指数爆炸，
栈深也可能爆栈。

**修复**：重写为 **BFS + `visited` 集合** 的迭代实现，最坏 O(N)，
`nil` 输入直接返回 `false`。

#### 需求 4：`InitRepo()` 作者硬编码 / 无 commit 兼容

**问题**：`InitRepo()` 强行用 `Git Agent` 身份创建初始 commit，
与 `SaveVersion` 要求真实作者的策略矛盾；且部分只需 `git init` 的场景
也被塞入了初始 commit。

**修复**：
- `InitRepo(authorName, authorEmail string)` 支持空作者参数。
- 有值：用真实身份创建初始 commit。
- 无值：仅执行 `git init`，不创建 commit。
- 所有依赖 HEAD 的接口在 `err == nil` 分支兜底空仓库。

#### 需求 5：`RestoreFile()` 加固

**问题**：
- 手写 IO 循环，混淆 EOF 与真实错误。
- 不保留可执行位、不识别符号链接。
- 直接 truncate 目标文件，中途失败会留下半写入的破损文件。

**修复**：
- 用 `io.Copy` 读全量。
- 识别 `filemode.Symlink`：`os.Remove` + `os.Symlink` 重建链接。
- 识别 `filemode.Executable`：目标 perm 设为 `0755`。
- 临时文件 + `os.Rename` 原子替换，中途失败清理临时文件。

#### 需求 17：见需求 1（合并处理）

---

### 🟢 低优 — 可维护性 / UX

#### 需求 6 / 7：清理重复工具函数 + sentinel error

**问题**：
- `agent.go` 手写 `contains` / `splitByComma` / `trimSpace` 等，
  而 `strings` 已导入。
- `gitwrapper.go` 用 `err.Error() == "..."` 精确比较，脆弱。

**修复**：
- 删除 `agent.go` 中的 4 个重复工具函数，全部替换为 `strings` 标准函数。
- `gitwrapper.go` 新增 sentinel error：
  - `ErrNoChangesToCommit`
  - `errLimitReached`
  - `errIterStop`
- 全部 4 处 `err.Error() ==` 精确比较替换为 `errors.Is()`。
- `SaveVersion` 兼容 go-git 新旧版本的空提交错误消息
  （`"no changes to commit"` / `"cannot create empty commit: clean working tree"`）。

#### 需求 14：`chatHistory` 上下文治理

**问题**：`chatHistory` 只在 `/clear` 时才清空，长期使用无限增长，
token 成本累积、上下文越界。

**修复**：
- 新增阈值常量：
  - `maxChatHistoryMessages = 60`
  - `maxChatMessageBytes = 16 * 1024`（单条 TextContent）
- `enforceChatHistoryLimits()`：
  - 保留所有 `system` 消息，从最旧的非 system 消息开始丢弃。
  - 单条 `TextContent` 超阈值时截断，保留头尾并标记 `...[truncated]...`。
- 关键写入点调用：用户输入后、ReAct 循环入口、工具结果回写后。

#### 需求 15：`intentToolMapping` 强类型化

**问题**：原实现用 `map[string][]string`，key 为硬编码字符串，
新增 `IntentType` 时容易漏更新且编译期无提示。

**修复**：
- `intentToolMapping` 改为 `map[interpreter.IntentType][]string`。
- 未命中意图时 `log.Printf("[intent-mapping] miss: %s", intent)` 告警，
  方便发现遗漏。

#### 需求 16：`formatVersionTable` 系统提示防泄漏

**问题**：拼接的 `[SYSTEM]` 规则文本会被 LLM 幻觉复述，直接泄漏给终端用户。

**修复**：
- 系统级规则改用 `<|system-note|>...<|/system-note|>` 标签包裹。
- 在标签内声明"禁止复述"，降低 LLM 复读概率。
- 前端渲染时可选择过滤该标签块。

#### 需求 19：CLI `/mode` 真正切换

**问题**：`/mode local` 只打印一句提示，实际未切换 `a.llmConfig.Enabled`，
`Process` 仍然走 LLM 路径。

**修复**：
- 新增 `Agent.SetLLMEnabled(bool) error`：
  - 加锁修改 `llmConfig.Enabled`。
  - 关闭 LLM 时释放对应资源（若适用）。
- `main.go` 的 `handleModeSwitch` 通过它真正切换模式。

---

## 四、单元测试补齐

**需求 21**：`gitwrapper.go`（56KB 核心逻辑）几乎无单元测试。

新增 [`internal/gitwrapper/gitwrapper_test.go`](../internal/gitwrapper/gitwrapper_test.go)
共 **10 个测试**，覆盖关键路径：

| 测试 | 覆盖点 |
|------|--------|
| `TestInitRepo_WithoutAuthor_NoInitialCommit` | 无作者时仅 `git init`，不创建初始 commit |
| `TestInitRepo_WithAuthor_CreatesInitialCommit` | 有作者时创建初始 commit，作者信息正确 |
| `TestStatus_StagedAndUnstagedAreIndependent` | 同一文件同时被暂存/工作区修改时，两个字段互不干扰 |
| `TestDiff_StagedOnlyFileProducesPatch` | 仅暂存修改的文件能产出 patch |
| `TestIsAncestor_MergeCommitDoesNotBlowUp` | diamond commit 图（多父提交）不指数爆炸 |
| `TestIsAncestor_NilInputs` | nil 输入直接返回 false，不 panic |
| `TestMerge_MissingUserConfigReturnsClearError` | user config 缺失时报明确错误，不产生"Git Agent"作者的脏 commit |
| `TestRestoreFile_PreservesExecutableBit` | 恢复文件时保留 `0755` 可执行位 |
| `TestRestoreFile_UnknownVersionDoesNotWipeFile` | 未知版本号时保留原文件，不留破损临时文件 |
| `TestSaveVersion_NoChangesReturnsSentinel` | 无变更时返回 `errors.Is(err, ErrNoChangesToCommit) == true` |

---

## 五、验收与回归

### 编译

```bash
go build ./...    # 通过（无输出）
```

### 全项目测试

```bash
go test -count=1 ./...
```

| 包 | 结果 |
|----|------|
| `internal/agent` | ✅ ok |
| `internal/conflict` | ✅ ok |
| `internal/gitwrapper` | ✅ ok |
| `internal/interpreter` | ✅ ok |
| `internal/planner` | ✅ ok |
| `internal/promptkit` | ✅ ok |
| `internal/web` | ✅ ok |

全绿，无回归。

---

## 六、遗留事项与后续建议

以下是本次没有落地、但建议后续跟踪的改进项：

1. **`RestoreFile` 短 hash 支持**：目前需要传完整 40 字符 SHA。
   `SaveVersion` / `GetHistory` 返回的都是短 hash（8 字符），
   建议 `RestoreFile` 内部做前缀匹配，提升 UX 一致性。
2. **Web 层单元/集成测试**：`internal/web/` 现有测试较薄，
   建议补齐 SSE 断连、Token 鉴权失败、`.git` 屏蔽等关键场景。
3. **`chatHistory` 持久化**：目前只在内存中，进程重启会丢失。
   若要支持长期会话，需要落盘（配合脱敏与加密）。
4. **Push 目标 URL 白名单可配置**：目前协议白名单在代码里硬编码，
   建议改为读配置，方便企业内网自签证书场景。
5. **Merge 三方合并的更细致策略**：目前仅覆盖三类冲突，
   `rename_edit` / `rename_rename` 尚未处理，重命名场景下会退化为
   `add + delete`。

---

## 变更快速跳转

- 计划：[requirements.md](../.codebuddy/plan/code-review-fixes/requirements.md) ｜ [task-item.md](../.codebuddy/plan/code-review-fixes/task-item.md)
- 关键源码变更：
  - [gitwrapper.go](../internal/gitwrapper/gitwrapper.go)
  - [gitwrapper_test.go](../internal/gitwrapper/gitwrapper_test.go)
  - [agent.go](../internal/agent/agent.go)
  - [redact.go](../internal/agent/redact.go)
  - [auth.go](../internal/web/auth.go)
  - [main.go](../main.go)
