# ADR-007: 将"不可抵赖"措辞改为分级表述，随能力兑现逐级升级

## Status
Proposed (2026-10-07)

## Background
README 与对比表声称 "Non-repudiable audit (signed receipts + hash chain)"。但当前实现：
- 默认临时密钥 → 签名不可跨运行归属（见 ADR-004）；
- 验证自引用 → 不证明来源（见 ADR-004）；
- 审计方 = 签名方 = 被审计方，且无第三方时间戳、无透明日志 → Ed25519 只证明"签名后未被篡改"，不证明"审计按声称方式执行过"；
- "hash chain" 实为 git 分支锚定，收据之间并未互相链接。

因此"不可抵赖"这一强断言在**当前实现下不成立**，属**过度声明（overclaim）**，是技术买家最先会挑的毛病。

## Decision
把断言改为**分级、可验证**的表述，并随能力兑现逐级升级：

| 级别 | 措辞 | 成立条件 |
|---|---|---|
| L0（当前真实能力） | "Tamper-evident, signed audit records"（防篡改的签名审计记录） | 已有签名；须配合 key pinning 才有意义 |
| L1 | "Attributable, timestamped signed receipts"（可归属、带时间戳的签名收据） | 完成 ADR-004 + ADR-005 |
| L2 | "Verifiable attestations（in-toto/DSSE）" | 完成 ADR-001 |
| L3 | "Non-repudiable（transparency-logged）" | 完成 ADR-006（+ 独立签名身份） |

- README/对比表**立即**从"Non-repudiable"降级为 L0/L1 的诚实表述，并删除/修正"hash chain"用词（改为"git-anchored"）。
- 每完成一级，才允许把措辞升级到对应级别。
- 对外文档不得声称尚未兑现的级别。

## Consequences
正面：
- 消除过度声明，经得起技术买家审视。
- 给出清晰的"说这句话需要先做什么"的路线图（P0→P1→P2）。
- 与开源诚信一致：一个卖可验证性的项目，自己的声明也必须可验证。

负面：
- 短期"营销强度"下降（但换来可信度）。
- 需要同步更新 README、对比表、白皮书、网站等多处文案。

## Related ADRs
ADR-001（in-toto）、ADR-004（持久密钥）、ADR-005（时间戳）、ADR-006（Rekor）
