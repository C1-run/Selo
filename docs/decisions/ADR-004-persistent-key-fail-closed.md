# ADR-004: 持久签名密钥 fail-closed，验证强制带外固定公钥

## Status
Proposed (2026-10-07)

## Background
当前实现有两处击穿核心卖点的问题：

1. **默认临时密钥**：`LoadSigningKey()`（`internal/receipt/sign.go:24-42`）在 `SELO_SIGNING_KEY` 未设置时，每进程生成临时 Ed25519 密钥，仅打印 stderr 警告。临时密钥意味着签名**不可跨运行归属到同一签名者**，"谁签的"这一主体不存在，抵赖/不可抵赖无从谈起。SECURITY.md 第 61 行把它列为"已知配置弱点"，但它正是 README"不可抵赖"卖点的地基。
2. **验证自引用**：`VerifyReceipt()`（`sign.go:107-130`）从收据**自身**读取 `public_key` 来验证**自身**签名。攻击者篡改内容后可重新生成密钥、重签、回写公钥，验证依旧通过。当前验证只证明"内部自洽"，**不证明来源真实性**。

## Decision
- **签名侧 fail-closed**：生产模式下 `SELO_SIGNING_KEY` 未设置即**拒绝签名/非零退出**。临时密钥仅允许在显式 `--dev` 或 `SELO_ALLOW_EPHEMERAL_KEY=1` 下使用，并在收据中标注 `key_mode: "ephemeral"`。
- **验证侧强制 key pinning**：`selo verify` 增加 `--pubkey <file|fingerprint>`。未提供时，输出**降级结论**："仅验证内部自洽，未验证来源真实性"，而非笼统 `VALID`。
- 收据新增 `key_mode` 字段（`persistent`/`ephemeral`），供下游策略判定。

## Consequences
正面：
- 恢复"签名可归属"，为一切"抵赖"语义提供主体。
- 堵住"篡改后重签即通过"的伪造路径。
- 使"不可抵赖"有资格被讨论（配合 ADR-005/006 才成立）。

负面：
- 破坏性变更：默认行为从"总能签名"变为"未配置即失败"，需在 CHANGELOG 标注并给出迁移指引。
- 用户需管理密钥（`selo keys generate`），增加一步配置。
- 现有收据无 `key_mode` 字段，需 schema 版本兼容处理。

## Related ADRs
ADR-005（时间戳）、ADR-006（Rekor）、ADR-007（措辞分级）
