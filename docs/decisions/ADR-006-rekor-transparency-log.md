# ADR-006: 收据锚定 Sigstore Rekor 透明日志

## Status
Proposed (2026-10-07)

## Background
即便有持久密钥与时间戳，签名方仍可能**对不同的验证方出示不同的收据**（equivocation / 双花式抵赖），或悄悄撤回某份收据。要证明"这是唯一的一份、且已被公开记录"，需要**只追加（append-only）、可独立验证**的透明日志。这正是"不可抵赖"的最后一块拼图。

## Decision
把收据的 DSSE 信封（或其哈希）提交 **Sigstore Rekor**：
- 用 `sigstore-go v1.3.0`（`pkg/tlog`）或 `rekor-cli` 上传，取回 `logIndex`、`integratedTime`、Merkle inclusion proof。
- 写入 `predicate.transparency.logIndex` / `.integratedTime`；可选封装为 **Sigstore bundle**（`.sigstore.json`）单文件。
- CLI 增加 `--rekor` 开关；失败时非零退出，**不得**产出"声称有 Rekor 锚"却无锚的 predicate。
- **隐私裁剪**：默认只上传**哈希或 DSSE 信封哈希**，不上传含任务名/文件路径的明文；有隐私需求时用私有 Rekor。

## Consequences
正面：
- 提供防 equivocation 的公开、只追加记录。
- `integratedTime` 兼作可信时间（部分替代 ADR-005）。
- 与 keyless 签名（ADR-002）天然配套。

负面：
- 依赖公网 Rekor（可用性、延迟）。
- 隐私风险：须默认只上传哈希，增加实现复杂度。
- 引入 `sigstore-go v1.3.0` 的重依赖，且要求项目 **Go 升级到 1.23+**（当前 `go.mod` 为 1.21）。
- 自建私有 Rekor 增加运维成本（本次 out-of-scope）。

## Related ADRs
ADR-002（keyless 签名）、ADR-004（持久密钥）、ADR-005（时间戳）、ADR-007（措辞分级）
