# Git Agent User Guide

> 🎯 This guide is for **absolute beginners** with no technical background. We'll walk you through using Git Agent to manage file versions step by step.

[中文文档](USAGE_zh.md) | [🔧 Tuning Guide](TUNING.md)

---

## Table of Contents

1. [What is Git Agent?](#1-what-is-git-agent)
2. [Installation & Startup](#2-installation--startup)
3. [Which Mode Should I Use?](#3-which-mode-should-i-use)
4. [First Time: 5-Minute Quick Start](#4-first-time-5-minute-quick-start)
5. [Common Operations](#5-common-operations)
   - [Save File Changes](#51-save-file-changes)
   - [View Change History](#52-view-change-history)
   - [Check Current Status](#53-check-current-status)
   - [See What Changed](#54-see-what-changed)
   - [Restore a Previous Version](#55-restore-a-previous-version)
   - [Tag a Version](#56-tag-a-version)
6. [Team Collaboration](#6-team-collaboration)
   - [Submit Changes to the Team](#61-submit-changes-to-the-team)
   - [View Colleagues' Changes](#62-view-colleagues-changes)
   - [Merge Colleagues' Changes](#63-merge-colleagues-changes)
   - [Get Latest Content](#64-get-latest-content)
7. [Workspaces (Branches)](#7-workspaces-branches)
   - [What is a Workspace?](#71-what-is-a-workspace)
   - [Create a Workspace](#72-create-a-workspace)
   - [Switch Workspace](#73-switch-workspace)
   - [List Workspaces](#74-list-workspaces)
8. [Conflict Resolution](#8-conflict-resolution)
   - [What is a Conflict?](#81-what-is-a-conflict)
   - [Detect Conflicts](#82-detect-conflicts)
   - [Resolve Conflicts](#83-resolve-conflicts)
9. [Special Commands](#9-special-commands)
10. [FAQ](#10-faq)
11. [Quick Reference](#11-quick-reference)

---

## 1. What is Git Agent?

Git Agent is a **file version management assistant**. Think of it this way:

> 📖 Imagine you're writing a report and every time you make changes, you save a copy ("report_v1.doc", "report_v2.doc", "report_final.doc"). Git Agent does this automatically — and better. Instead of creating a pile of files, it quietly records every change, and you can view history, compare differences, or restore any previous version at any time.

**Key Benefits:**

| Scenario | Without Git Agent | With Git Agent |
|----------|-------------------|----------------|
| File broken, want to revert | Old version is gone 😱 | Restore with one command ✅ |
| Want to know who changed what | Have to ask everyone | View change log with one command ✅ |
| Multiple people editing simultaneously | Files overwrite each other 😱 | Auto-detect and help resolve ✅ |
| Want to compare two versions | Manually open two files | View diff with one command ✅ |

---

## 2. Installation & Startup

### 2.1 Prerequisites

- Your computer needs **Go** (version 1.24 or higher)
  - To check: open a terminal and type `go version` — if it shows a version number, you're good
  - If not installed, visit https://go.dev/dl/ to download

### 2.2 Download the Project

Open a terminal (on macOS, search for "Terminal" in Launchpad), and enter:

```bash
git clone <repository-url> git-agent
cd git-agent
go mod tidy
```

### 2.3 Starting Up

#### Option 1: Local Mode (recommended for beginners)

No extra configuration needed, just run:

```bash
go run main.go
# or: make dev
```

#### Option 2: LLM Mode (smarter, requires API Key)

If you have an LLM API Key (e.g., OpenAI, DeepSeek), you can use the smarter mode:

```bash
# Using OpenAI
go run main.go --api-key sk-your-key --model gpt-4o

# Using DeepSeek
go run main.go --api-key sk-your-key --base-url https://api.deepseek.com/v1 --model deepseek-chat

# Using Azure OpenAI
go run main.go --api-key your-key --base-url https://your-name.openai.azure.com/openai/deployments/your-model --model gpt-4o

# Using Local Ollama (no cloud API needed!)
go run main.go --api-key ollama --base-url http://localhost:11434/v1 --model qwen2.5:7b
```

> 💡 **Tip**: You can also use environment variables instead of command-line arguments, so you don't have to type them every time:
> ```bash
> export GIT_AGENT_API_KEY=sk-your-key
> export GIT_AGENT_BASE_URL=https://api.deepseek.com/v1
> export GIT_AGENT_MODEL=deepseek-chat
> go run main.go
> ```

#### 🦙 Using Local Ollama (Free, No Cloud Required)

If you have [Ollama](https://ollama.ai) installed locally, you can run Git Agent with a local LLM — completely free and offline!

**Step 1: Install Ollama**

Visit https://ollama.ai to download and install Ollama for your platform.

**Step 2: Pull a model**

```bash
# Recommended: Qwen2.5 (best Function Calling support)
ollama pull qwen2.5:7b

# Alternative: Llama 3.1
ollama pull llama3.1:8b
```

**Step 3: Start Git Agent**

```bash
go run main.go --api-key ollama --base-url http://localhost:11434/v1 --model qwen2.5:7b
```

Or using environment variables:

```bash
export GIT_AGENT_API_KEY=ollama
export GIT_AGENT_BASE_URL=http://localhost:11434/v1
export GIT_AGENT_MODEL=qwen2.5:7b
go run main.go
```

> ⚠️ **Important Notes for Ollama Users**:
> - The `--api-key` can be any non-empty string (e.g., `ollama`) — it's only used to enable LLM mode. Ollama doesn't require real authentication.
> - **Function Calling support varies by model**: `qwen2.5` has the best support; `llama3.1` has basic support. Models without Function Calling support (e.g., `mistral`, `gemma`) may cause tool invocation failures.
> - Make sure Ollama is running (`ollama serve`) before starting Git Agent.
> - Local models are slower than cloud APIs — responses may take a few seconds depending on your hardware.

#### Successful Startup

After starting, you'll see the welcome screen:

```
Git Agent v0.1.0(abc1234)
  🧠 LLM deepseek-chat @ api.deepseek.com

  输入「帮助」查看所有操作  输入「退出」结束会话


🧠 > _
```

> 📝 In local mode, the header will show `📝 Local Mode (configure LLM for smarter experience: git-agent --help)`. Usage is exactly the same, but comprehension is more limited.

---

## 3. Which Mode Should I Use?

| Feature | 📝 Local Mode | 🧠 LLM Mode |
|---------|--------------|-------------|
| **API Key Required** | ❌ No | ✅ Yes |
| **Natural Language Understanding** | Basic keyword matching | Deep semantic understanding |
| **Expression Flexibility** | Must use common phrases | Say it however you want |
| **Conversation Ability** | None | Multi-turn conversations |
| **Error Handling** | Simple prompts | Intelligent explanations and guidance |
| **Cost** | Free | Pay per API usage (or free with local Ollama) |
| **Recommended For** | First trial, simple operations | Daily use, complex needs |

> 💡 **Suggestion**: Start with local mode to get familiar with the basics, then switch to LLM mode when you need more intelligence.

---

## 4. First Time: 5-Minute Quick Start

Let's walk through a complete example to get you started from scratch.

### Step 1: Start Git Agent

```bash
cd your-working-directory
go run main.go
```

### Step 2: Initialize the Repository

When using Git Agent in a new folder for the first time, you need to initialize:

```
🧠 > Initialize repository

✅ Repository created. You can now start adding files.
```

> 💡 This is like getting a new folder ready for storing and managing your documents.

### Step 3: Add Some Files

While Git Agent is running, create or edit files in the directory using your favorite tools (Word, Notepad, Excel, etc.):

- `market-report.md` — A market analysis report
- `data.xlsx` — A data spreadsheet

### Step 4: Save Your First Version

```
🧠 > Save changes, this is the first draft

✅ Saved as new version #a1b2c3d4
  💡 You might also want to:
     • Push to remote repository
     • View change history
```

> 💡 It's like taking a "snapshot" of all your current files — you can always return to this state later.

### Step 5: Make Some Changes

Edit `market-report.md` with your editor, for example adding a paragraph of analysis.

### Step 6: See What Changed

```
🧠 > See what changed

📋 Change details:
File: market-report.md | Status: Modified (unstaged)
```

### Step 7: Save the New Version

```
🧠 > Save changes, added competitive analysis section

✅ Saved as new version #e5f6a7b8
```

### Step 8: View Change History

```
🧠 > View history

📋 Found 2 history records:
  1. Version #e5f6a7b8
  2. Version #a1b2c3d4
```

🎉 **Congratulations!** You've mastered the basics of Git Agent!

---

## 5. Common Operations

### 5.1 Save File Changes

This is the most common operation. After editing files, just tell the Agent to save.

**You can say:**

| Phrase | Effect |
|--------|--------|
| `Save changes` | Save all modified files |
| `Save changes, updated chapter 3` | Save all changes with description "updated chapter 3" |
| `Save` | Save all modified files |
| `Commit` | Save all modified files |
| `Stage` | Save all modified files |

**Full Example:**

```
🧠 > Save changes, completed first draft of market analysis

✅ Saved as new version #c3d4e5f6
  💡 You might also want to:
     • Push to remote repository
     • View change history
```

**Save Only Specific Files:**

```
🧠 > Save changes to market-report.md, adjusted data section

✅ Saved as new version #d4e5f6a7
```

> ⚠️ **Note**: If there are no changes, you'll see "Nothing to save". Make sure to edit files first before saving.

---

### 5.2 View Change History

View all previously saved version records.

**You can say:**

| Phrase | Effect |
|--------|--------|
| `View history` | Show the last 10 change records |
| `View change log` | Same as above |
| `See who changed what` | Show all change records (with author info) |
| `History` | Same as above |

**Full Example:**

```
🧠 > View history

📋 Found 5 history records:
  1. Version #f7a8b9c0
  2. Version #e5f6a7b8
  3. Version #d4e5f6a7
  4. Version #c3d4e5f6
  5. Version #a1b2c3d4
  💡 You might also want to:
     • Restore a version
     • View specific differences
```

---

### 5.3 Check Current Status

See which files have been modified, which are new, and which have already been saved.

**You can say:**

| Phrase | Effect |
|--------|--------|
| `Check status` | Show current change status |
| `Current status` | Same as above |
| `What changed` | Same as above |
| `Any changes` | Same as above |
| `What's the situation` | Same as above |

**Full Example (with changes):**

```
🧠 > Check status

📋 Current status:
  Staged changes: 2 files
  Unstaged changes: 1 file
  New files: 1

📡 Local is ahead of remote by 2 commits. Use "push" to sync.
```

**Full Example (no changes):**

```
🧠 > Check status

✅ No unsaved changes

📡 Local and remote are in sync.
```

---

### 5.4 See What Changed

View the specific content of your changes, compared to the last saved version.

**You can say:**

| Phrase | Effect |
|--------|--------|
| `View diff` | Show modification summary for all files |
| `See what changed` | Same as above |
| `Compare differences` | Same as above |
| `What are the changes` | Same as above |
| `What changed in market-report.md` | Show changes for a specific file only |
| `What changed in commit 5c1a42e1` | Show changes in a specific commit |

**Full Example:**

```
🧠 > See what changed

📋 Change details:
File: market-report.md | Status: Modified (unstaged)
File: data.xlsx | Status: Modified (unstaged)
```

**View changes in a specific commit:**

```
🧠 > What changed in commit 5c1a42e1?

📋 Changes in commit 5c1a42e1:
File: agent.go | +45 -12
File: tools.go | +18 -3
```

---

### 5.5 Restore a Previous Version

If you've broken a file or want to go back to a previous version, use the restore function.

**You can say:**

| Phrase | Effect |
|--------|--------|
| `Restore version` | Restore to a previous version (need version number) |
| `Go back to previous version` | Same as above |
| `Rollback` | Same as above |
| `Undo changes` | Same as above |
| `Restore version a1b2c3d4` | Restore to a specific version |

> ⚠️ **Important**: Restoring the entire repository is an **irreversible operation** — unsaved changes will be lost! We recommend saving the current version first, then restoring.

**Full Example:**

```
🧠 > Restore version e5f6a7b8

✅ Restored to specified version
```

**Restore Only a Specific File:**

```
🧠 > Restore market-report.md to version e5f6a7b8

✅ Restored to specified version
```

> 💡 **Tip**: Use "View history" first to find the version number, then use "Restore version" to go back.

---

### 5.6 Tag a Version

Give an important version a memorable name like "final" or "v1.0" for easy lookup later.

**You can say:**

| Phrase | Effect |
|--------|--------|
| `Tag v1.0` | Tag the current version as v1.0 |
| `Tag version final` | Tag the current version as "final" |
| `Create tag` | Create a tag (need to provide tag name) |

**Full Example:**

```
🧠 > Tag v1.0

✅ Version tagged as "v1.0"
```

> 💡 Best used at milestones: tag "review" before submitting for review, "final" after approval.

---

## 6. Team Collaboration

### 6.1 Submit Changes to the Team

Push your changes to the remote repository so team members can see them.

**You can say:**

| Phrase | Effect |
|--------|--------|
| `Submit to team` | Save and push all changes |
| `Submit for review` | Same as above |
| `Request merge` | Same as above |

**Full Example:**

```
🧠 > Submit to team, completed competitive analysis

✅ Synced to remote repository
```

#### Authentication for Remote Push

When pushing to a remote repository for the first time, you may need to authenticate. Git Agent supports two protocols, each with a different authentication method:

| Protocol | URL Format | Authentication Method | Best For |
|----------|-----------|----------------------|----------|
| **HTTPS** | `https://github.com/user/repo.git` | Username + Access Token | Beginners (recommended) |
| **SSH** | `git@github.com:user/repo.git` | SSH Key | Experienced users |

> 💡 **For new repositories, we recommend using HTTPS + Access Token** — it's the simplest setup for beginners.

**HTTPS Authentication (Recommended for Beginners):**

If push fails with an authentication error, Git Agent will guide you through getting an access token. Here's how it works:

1. **Git Agent detects auth failure** and prompts you with clear instructions
2. **You get an access token** from your Git hosting platform (e.g., GitHub)
3. **You tell Git Agent your username and token** in natural language
4. **Git Agent retries the push** with your credentials

**How to get a GitHub Personal Access Token:**

1. Log in to GitHub → Click your avatar (top right) → **Settings**
2. Scroll down → **Developer settings** → **Personal access tokens** → **Tokens (classic)**
3. Click **Generate new token (classic)**
4. Give it a description, check the **repo** scope, and click **Generate token**
5. **Copy the token** (you won't be able to see it again!)

Then tell Git Agent:

```
🧠 > My username is jackz-jones and my token is ghp_xxxxx

✅ Pushed successfully with your credentials!
```

**Switching from SSH to HTTPS:**

If your repository was cloned with SSH and you're having trouble with SSH keys, you can switch to HTTPS:

```
🧠 > Push to remote, use HTTPS address https://github.com/jackz-jones/my-project.git, username jackz-jones, token ghp_xxxxx

✅ Remote URL switched to HTTPS and pushed successfully!
```

**SSH Authentication (For Experienced Users):**

Git Agent automatically tries to use your SSH keys in this order:

1. **SSH config `IdentityFile`** — If you've configured `~/.ssh/config` with an `IdentityFile` for the host
2. **SSH Agent** — If you've loaded keys into ssh-agent (`ssh-add ~/.ssh/id_rsa`)
3. **Default key files** — `~/.ssh/id_ed25519`, `~/.ssh/id_rsa`, `~/.ssh/id_ecdsa`

If all SSH authentication methods fail, Git Agent will suggest switching to HTTPS (easier for most users).

**Environment Variables for HTTPS Credentials:**

You can also set credentials via environment variables to avoid entering them each time:

```bash
export GIT_HTTP_USERNAME=your-username
export GIT_HTTP_PASSWORD=ghp_your-token
go run main.go
```

> ⚠️ **Prerequisite**: You need to configure the remote repository URL first. For new repositories, you also need to "push" once.

---

### 6.2 View Colleagues' Changes

See what changes team members have made recently.

**You can say:**

| Phrase | Effect |
|--------|--------|
| `See what Alex changed` | View Alex's change history |
| `See colleagues' changes` | View everyone's change history |
| `Team changes` | Same as above |
| `What did others change` | Same as above |

**Full Example:**

```
🧠 > See what Alex changed

📋 Alex's recent changes:
  1. [3 hours ago] Updated market data
  2. [Yesterday] Adjusted conclusion section
  3. [2 days ago] Added references
```

> 💡 In local mode, names like "Alex", "Bob", "Chris" will be recognized as author filters. In LLM mode, you can use any person's name.

---

### 6.3 Merge Colleagues' Changes

Merge a colleague's workspace into your current work.

**You can say:**

| Phrase | Effect |
|--------|--------|
| `Merge Alex's changes` | Merge a specified workspace |
| `Approve merge feature-branch` | Merge a specified workspace |
| `Approve merge` | Merge (need to specify branch name) |

**Full Example:**

```
🧠 > Merge changes from alex-market-analysis

✅ Merged alex-market-analysis changes into current workspace
```

> ⚠️ If both parties modified the same file, a **conflict** may occur. See [Conflict Resolution](#8-conflict-resolution).

---

### 6.4 Get Latest Content

Pull the latest changes from the remote repository made by other team members.

**You can say:**

| Phrase | Effect |
|--------|--------|
| `Pull` | Get latest content from remote |
| `Update` | Same as above |
| `Sync` | Same as above |
| `Get latest` | Same as above |
| `Download latest` | Same as above |

**Full Example:**

```
🧠 > Get latest

✅ Latest content retrieved
```

---

## 7. Workspaces (Branches)

### 7.1 What is a Workspace?

> 🏢 **In Plain English**: A workspace is like a "personal desk" in an office.
>
> Imagine you and a colleague are both editing a report. If you edit the same file directly, you'll interfere with each other. A workspace gives each person their own independent editing space — when everyone is done, the changes can be merged together.

**Common Use Cases:**
- Start a new proposal without affecting the main version → Create a workspace
- Try out a new idea without committing to it → Create a workspace
- Multiple people editing different parts → Each person creates a workspace

---

### 7.2 Create a Workspace

**You can say:**

| Phrase | Effect |
|--------|--------|
| `Create workspace new-proposal` | Create a workspace named "new-proposal" |
| `Create branch feature-market` | Create a workspace named "feature-market" |
| `New branch experiment` | Create a workspace named "experiment" |

**Full Example:**

```
🧠 > Create workspace new-proposal

✅ Workspace "new-proposal" created
```

---

### 7.3 Switch Workspace

Switch between different workspaces.

**You can say:**

| Phrase | Effect |
|--------|--------|
| `Switch to new-proposal` | Switch to the "new-proposal" workspace |
| `Switch to main` | Switch to the main workspace |
| `Go to feature-market` | Switch to the specified workspace |

**Full Example:**

```
🧠 > Switch to new-proposal

✅ Switched to workspace "new-proposal"
```

> 💡 Make sure your current changes are saved before switching, otherwise unsaved changes may be lost.

---

### 7.4 List Workspaces

**You can say:**

| Phrase | Effect |
|--------|--------|
| `List workspaces` | Show all workspaces |
| `Branch list` | Same as above |
| `List branches` | Same as above |

**Full Example:**

```
🧠 > List workspaces

📋 Branches: [main new-proposal feature-market]
```

---

## 8. Conflict Resolution

### 8.1 What is a Conflict?

> ⚡ **In Plain English**: A conflict is when two people modified the same part of the same file.
>
> For example, you changed a paragraph to "Plan A", and your colleague changed the same paragraph to "Plan B". Git Agent can't decide which one to keep — that's where you come in.

**Typical Conflict Scenarios:**
1. You and a colleague modified the same position in the same file
2. You modified a file while a colleague deleted the same file
3. You and a colleague both added files with the same name

---

### 8.2 Detect Conflicts

Proactively check for conflicts.

**You can say:**

| Phrase | Effect |
|--------|--------|
| `Detect conflicts` | Scan for conflicts |
| `Any conflicts` | Same as above |
| `Conflicts` | Same as above |

**Full Example (no conflicts):**

```
🧠 > Any conflicts

📋 Conflict check result: [] No conflicts found
```

**Full Example (with conflicts):**

```
🧠 > Any conflicts

📋 Conflict check result:
  📄 report.md: You and a colleague both modified the same position
```

---

### 8.3 Resolve Conflicts

There are three conflict resolution strategies:

| Strategy | Description | When to Use |
|----------|-------------|-------------|
| **ours** | Keep my changes | When your version is more accurate |
| **theirs** | Use their changes | When the colleague's version is more complete |
| **merge** | Auto-merge both changes | When changes don't overlap and can be combined |

**You can say:**

| Phrase | Effect |
|--------|--------|
| `Resolve conflict using ours` | Keep your version for conflict files |
| `Resolve conflict using theirs` | Use their version for conflict files |
| `Resolve conflict using merge` | Auto-merge conflict files |

**Full Example (LLM Mode):**

```
🧠 > Pull latest changes

⚠️ Found 1 conflict to resolve:
  📄 report.md: You and a colleague both modified the same position
  💡 Suggestion: The conflict area is simple, auto-merge is recommended

🧠 > Resolve conflict using merge strategy

✅ Conflict resolved!
  📝 report.md: Both changes have been auto-merged
  💡 You might also want to:
     • Save the merge result
     • Submit to team
```

**Full Example (Local Mode):**

```
📝 > Resolve conflict report.md using merge

✅ Both changes have been auto-merged
```

> 💡 **Suggestion**: If you're not sure which strategy to choose, try `merge` first. If the auto-merge isn't satisfactory, you can always manually edit the file.

---

## 9. Special Commands

In interactive mode, besides natural language operations, the following special commands are supported:

| Command | Description | Example |
|---------|-------------|---------|
| `/mode local` | Switch to local mode | `/mode local` |
| `/mode llm` | Switch to LLM mode | `/mode llm` |
| `/clear` | Clear conversation history | `/clear` |
| `/reset` | Clear conversation history (same as above) | `/reset` |
| `exit` | Exit the program | `exit` |
| `quit` | Exit the program | `quit` |
| `help` | View help information | `help` |

**Clear Conversation History Example:**

```
🧠 > /clear

🧹 Conversation history cleared
```

> 💡 `/clear` only clears the conversation record — it does NOT delete any files or versions.

---

## 10. FAQ

### Q1: What if I see "Repository not initialized" after starting?

**Answer**: Type `Initialize repository` in Git Agent. This will create a document repository in the current directory.

```
🧠 > Initialize repository

✅ Repository created. You can now start adding files.
```

---

### Q2: Why does it say "Nothing to save" when I try to save?

**Answer**: This means your files haven't changed since the last save. Please check:
1. Did you actually edit and save the file? (Some editors require Ctrl+S / Cmd+S to save)
2. Is the file you edited in the directory where Git Agent is running?

---

### Q3: Is there a difference between local mode and LLM mode in what I can say?

**Answer**: Local mode requires **common phrases** (like "Save changes", "View history"), while LLM mode is much more flexible. Compare:

| What you want to say | Local Mode | LLM Mode |
|---------------------|------------|----------|
| `Save changes` | ✅ Recognized | ✅ Recognized |
| `Help me save the stuff I changed` | ❌ May not recognize | ✅ Understands |
| `I want to go back to yesterday's version` | ❌ Can't process | ✅ Understands |
| `I don't want the report I just changed, switch it back` | ❌ Can't process | ✅ Understands |

---

### Q4: After restoring a version, can I go back to the latest version?

**Answer**: Yes! Restoring a version does NOT delete the history — you can restore to any version at any time. But **save your current changes first** before restoring, otherwise unsaved content will be lost.

---

### Q5: What does "Token usage" mean in LLM mode?

**Answer**: Tokens are the unit that LLMs use to count text — similar to "word count". Every LLM conversation consumes tokens. The usage shown in the output helps you understand the cost of each operation. For example:

```
  Token 用量：256（输入: 180, 输出: 76）
```

- `输入 (prompt)`: The amount of text sent to the LLM
- `输出 (completion)`: The amount of text the LLM replied with
- Cost = Total tokens × Price per token (varies by model)

---

### Q6: How do I set my name and email?

**Answer**: You can set them via environment variables before starting Git Agent:

```bash
export GIT_AGENT_USER=Alex
export GIT_AGENT_EMAIL=alex@company.com
go run main.go
```

Or you can tell Git Agent directly in conversation:

```
🧠 > My name is Alex and my email is alex@company.com

✅ User info updated: Alex <alex@company.com>
```

This way, when you save a version, the author will show as "Alex".

---

### Q7: How do I use a local Ollama model?

**Answer**: First make sure Ollama is installed and running, then pull a model and start Git Agent:

```bash
# 1. Start Ollama service
ollama serve

# 2. Pull a model (recommended: qwen2.5 for best Function Calling support)
ollama pull qwen2.5:7b

# 3. Start Git Agent
go run main.go --api-key ollama --base-url http://localhost:11434/v1 --model qwen2.5:7b
```

> ⚠️ The `--api-key` must be non-empty to enable LLM mode, but Ollama doesn't require a real key. Any string like `ollama` works.

---

### Q8: Ollama model doesn't call tools / Function Calling fails

**Answer**: Not all Ollama models support Function Calling equally well. Try these steps:

1. **Switch to a better-supported model** — `qwen2.5:7b` or `qwen2.5:14b` have the best support
2. **Update Ollama** — older versions may have incomplete OpenAI compatibility: `ollama update`
3. **Check if the model supports tools** — some models (like `mistral`, `gemma`, `phi`) have limited or no Function Calling support

If Function Calling still fails, you can fall back to local mode by removing the `--api-key` flag.

---

### Q9: How do I view help?

Type `help` in interactive mode:

```
🧠 > help

📂 Version Control: Save changes | View history | Restore version | View diff
👥 Team Collaboration: Submit to team | See what XX changed | Merge XX's proposal
🔧 Repository Management: Initialize repo | Check status | Push | Pull
👤 User Settings: My name is XX | My email is XX

Just type in natural language!
```

---

### Q10: Push failed with authentication error — what should I do?

**Answer**: This means the remote repository requires authentication. Git Agent will guide you through the process. Here are the two scenarios:

**Scenario A: Your remote uses HTTPS (URL starts with `https://`)**

Git Agent will prompt you to provide a username and access token. Just tell it in natural language:

```
🧠 > Push with username jackz-jones and token ghp_xxxxx
```

To get a GitHub Personal Access Token: Login → Avatar → Settings → Developer settings → Personal access tokens → Tokens (classic) → Generate new token → Check "repo" scope → Generate → Copy token.

**Scenario B: Your remote uses SSH (URL starts with `git@`)**

If SSH authentication fails, Git Agent will suggest switching to HTTPS, which is easier:

```
🧠 > Push using HTTPS address https://github.com/user/repo.git, username jackz-jones, token ghp_xxxxx
```

Or if you prefer to fix SSH, make sure:
1. Your SSH key is loaded: `ssh-add ~/.ssh/id_rsa`
2. Your `~/.ssh/config` has the correct `IdentityFile` for the host

> 💡 **For beginners**, we recommend switching to HTTPS — it's much simpler!

---

## 11. Quick Reference

### Version Management

| What you want to do | Local Mode Phrase | LLM Mode Phrase (more flexible) |
|---------------------|-------------------|--------------------------------|
| Save files | `Save changes` | `Help me save`, `Save, updated chapter 3` |
| View history | `View history` | `Show me the change log`, `What was changed before` |
| Check status | `Check status` | `What's the current situation`, `Any unsaved changes` |
| View diff | `View diff` | `What changed`, `Compare for me` |
| Restore version | `Restore version` | `Go back to the previous version`, `Undo my last change` |
| Create tag | `Tag v1.0` | `Mark as final`, `Tag this version as v2` |

### Team Collaboration

| What you want to do | Local Mode Phrase | LLM Mode Phrase (more flexible) |
|---------------------|-------------------|--------------------------------|
| Submit to team | `Submit to team` | `Send my changes for review` |
| View colleague's changes | `See what Alex changed` | `What has Alex changed recently` |
| Merge changes | `Merge Alex's changes` | `Merge Alex's proposal into mine` |
| Get latest | `Pull` | `Sync the latest content`, `Update` |
| Push changes | `Push` | `Upload to remote`, `Sync to server` |

### Workspaces

| What you want to do | Local Mode Phrase | LLM Mode Phrase (more flexible) |
|---------------------|-------------------|--------------------------------|
| Create workspace | `Create workspace new-proposal` | `Open a new workspace for the proposal` |
| Switch workspace | `Switch to new-proposal` | `Move to the new-proposal side` |
| List workspaces | `List workspaces` | `How many workspaces are there` |

### Conflict Resolution

| What you want to do | Local Mode Phrase | LLM Mode Phrase (more flexible) |
|---------------------|-------------------|--------------------------------|
| Detect conflicts | `Detect conflicts` | `Any conflicts`, `Check for conflicts` |
| Keep mine | `Resolve conflict ours` | `Use my version for the conflict` |
| Use theirs | `Resolve conflict theirs` | `Use their version for the conflict` |
| Auto-merge | `Resolve conflict merge` | `Auto-merge the conflict` |

### Special Commands

| Command | Description |
|---------|-------------|
| `help` | View help |
| `/mode local` | Switch to local mode |
| `/mode llm` | Switch to LLM mode |
| `/clear` | Clear conversation |
| `exit` / `quit` | Exit program |

---

## Environment Variables Reference

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `GIT_AGENT_API_KEY` | LLM API Key | None | `sk-xxxxx`, `ollama` (for local Ollama) |
| `GIT_AGENT_BASE_URL` | LLM API URL | `https://api.openai.com/v1` | `https://api.deepseek.com/v1`, `http://localhost:11434/v1` (Ollama) |
| `GIT_AGENT_MODEL` | LLM Model Name | `gpt-4o` | `deepseek-chat`, `qwen2.5:7b` (Ollama) |
| `GIT_AGENT_MAX_TOKENS` | Max Tokens | `4096` | `8192` |
| `GIT_AGENT_USER` | Username (**required**) | — | `Alex` |
| `GIT_AGENT_EMAIL` | User Email (**required**) | — | `alex@company.com` |
| `GIT_HTTP_USERNAME` | HTTPS Git Username (for push auth) | None | `jackz-jones` |
| `GIT_HTTP_PASSWORD` | HTTPS Git Password/Token (for push auth) | None | `ghp_xxxxx` |

---

> 📞 **Having Issues?** Type `help` in interactive mode for built-in help, or contact your administrator for support.