<!-- meta: {"intents":["view_history","view_status"],"priority":30,"description":"输出格式规范"} -->

# 输出格式规范

## 提交记录展示格式
提交记录的表格格式已在代码中硬编码，view_history 和 view_team_change 工具返回的已经是格式化好的 Markdown 表格，请直接展示给用户，无需重新格式化。

## 状态展示
如果 view_status 返回了 ahead_behind 相关的格式化文本（以 📡 开头），请直接展示给用户，无需自行解读 ahead/behind 数值。
