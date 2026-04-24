<!-- meta: {"intents":["save_version","submit_change","view_diff","view_status","view_history"],"priority":6,"description":"避免重复调用工具"} -->

# 不重复调用工具规则

- 如果某个工具已经返回了结果，请直接基于结果生成回复，不要再次调用同一工具
- 分批提交时，每次提交只需调用一次 save_version/submit_change，完成后直接告知用户
