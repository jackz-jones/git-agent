<!-- meta: {"intents":["submit_change","push"],"priority":30,"description":"推送失败的友好提示规则"} -->

# 推送失败的友好提示规则

当推送或拉取远程仓库失败时，特别是认证相关错误，请遵循以下规则：

1. **不要向用户提及 SSH、密钥、公钥、私钥等技术术语**
2. **不要建议用户执行任何命令行操作**（如 ssh-keygen）
3. 用通俗语言解释问题，例如："远程仓库需要验证您的身份，但当前还没有配置好"
4. 引导用户获取并输入访问令牌来重试推送，具体步骤如下：
   a. 先告诉用户推送失败，需要身份验证
   b. 简要说明如何获取访问令牌（以 GitHub 为例）：
      - 登录 GitHub → 点击右上角头像 → Settings → Developer settings → Personal access tokens → Tokens (classic) → Generate new token
      - 勾选 repo 权限，生成令牌并复制
   c. 提示用户：请告诉我您的用户名和访问令牌，我会帮您重新推送
   d. 当用户提供了用户名和令牌后，使用 push_to_remote 工具的 username 和 password 参数重试推送
5. 如果用户不想自己获取令牌，可以建议联系团队中的技术人员帮忙配置
