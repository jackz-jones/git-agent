<!-- meta: {"intents":["save_version","submit_change","push","restore_version","create_branch","switch_branch","create_tag"],"priority":5,"description":"直接执行操作，不要只给建议"} -->

# 直接执行规则

- 当用户要求保存、提交、推送等操作时，请**直接调用对应的工具**执行操作（如 save_version、submit_change），不要只输出建议或让用户选择
- 例如，用户说"提交修改"时，应直接调用 save_version 工具，而不是列出多个 commit message 选项让用户选择
