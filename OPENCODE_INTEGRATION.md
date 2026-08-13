# C1-Forge ↔ OpenCode 集成

## 架构

```
OpenCode 生成/修改代码
       │
       ▼
c1-forged check --steps-file steps.json --format json
       │
       ▼
AggregateVerdict { action: pass/review/stop, blocking_tools, warnings }
       │
       ▼
反馈回 OpenCode 对话流
```

## CLI 命令

```bash
# pass 场景（exit 0）
c1-forged check --steps-file samples/steps-pass.json

# review 场景（exit 0）
c1-forged check --steps-file samples/steps-review.json

# block 场景（exit 2）
c1-forged check --steps-file samples/steps-block.json

# JSON 输出
c1-forged check --steps-file samples/steps-block.json --format json
```

## OpenCode 自举 Prompt

将以下 Prompt 发给 OpenCode，让其自动生成适配自身的合规工具：

```
为 OpenCode 创建一个自定义工具，名称叫 `c1_forge_compliance_check`。该工具接收一个文件路径列表作为输入，然后调用本地已安装的 `c1-forged` CLI 命令，对这些文件进行合规检查。

CLI 调用命令示例：
c1-forged check --steps-file <path> --format json

解析返回的 JSON 对象（AggregateVerdict 类型，包含 action, final_status, stop_required, review_required, blocking_tools, warnings 等字段），并根据以下规则反馈给 OpenCode 对话流：
- 如果 action 是 "stop"，则在对话中显示红色警告："合规检查失败：部署已被阻断。"并列出所有 warnings 和 blocking_tools。
- 如果是 "review"，则显示黄色提示："需要人工审核，请检查以下警告：<warnings>"
- 如果是 "pass"，仅显示绿色简短确认。

将该工具注册为自动触发钩子，在每次 OpenCode 对文件进行修改后执行。
```

## 提供的样本文件

| 文件 | 场景 | 预期 action | 包含 |
|------|------|:----------:|------|
| `samples/steps-pass.json` | 全绿 | pass | 4 pass 步骤，P45/CT/Redline/tuttut |
| `samples/steps-review.json` | 混合 | review | 1 warn（Redline high risk） |
| `samples/steps-block.json` | 阻断 | stop | 1 block（P45 scope 违规）+ 2 warn |
