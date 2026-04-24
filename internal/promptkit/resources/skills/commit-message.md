<!-- meta: {"intents":["save_version","submit_change"],"priority":10,"description":"Commit message 撰写规范"} -->

# Commit Message 撰写规范

当执行 save_version 或 submit_change 操作时，message 参数必须遵循以下规则：

1. **必须使用英文**撰写 commit message
2. 使用 conventional commit 风格，格式为 "type: summary"
3. **type 类型选择规则**（按优先级从高到低，只能选一个最匹配的）：
   - "fix": 修复 bug
   - "feat": 源码文件变更（涉及 *.go、*.py、*.ts、*.cpp 等源码文件）
   - "refactor": 代码重构（不改变功能逻辑）
   - "perf": 性能优化
   - "docs": 文档变更（仅当修改的都是 *.md、*.txt 等纯文档文件时使用；包含源码文件时用 feat）
   - "style": 代码格式调整
   - "test": 测试用例
   - "chore": 构建/辅助工具变动
   **关键原则：判断 type 依据是被修改的文件类型，而非修改目的。只要包含源码文件就应使用 feat。**
4. **summary 撰写要求**：
   - 使用祈使句（如 add 而非 added）
   - **必须具体描述改了什么**，不要笼统说"update files"
5. **示例**：
   - ✅ fix: resolve nil pointer error in Diff method
   - ✅ feat: add commit_hash parameter to view_diff
   - ✅ docs: add CLI usage guide to USAGE.md
   - ❌ feat: update source code files（太笼统）
   - ❌ chore: save changes（没说改了什么）
