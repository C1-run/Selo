# ADR-001: 采用 in-toto attestation（Statement v1 + DSSE）作为收据导出格式

## Status
Proposed (2026-10-07)

## Background
Selo 的收据目前是私有 JSON（`ForgeReceipt`，`internal/receipt/receipt.go`），只有 `selo verify` 能解释。第三方要验证收据，必须安装 Selo 并理解其私有 schema。这与"任何人可验证"的主张冲突，也无法接入主流供应链验证工具（cosign、slsa-verifier、policy engine）。需要一个标准、可互操作、带明确验证语义的封装格式。

## Decision
将收据**导出**为 in-toto attestation：
- Statement `_type` 固定为 `https://in-toto.io/Statement/v1`（in-toto attestation framework spec v1.0.2）。
- `predicateType` 使用自定义、可解引用、带版本的 `https://selo.c1.run/attestation/v1`。
- 用 **DSSE** 信封包裹（`payloadType: application/vnd.in-toto+json`），签名对象为 PAE。
- 现有 `receipt.json` 格式**保持不变**，导出是**新增能力**（`selo receipt export --format in-toto`），向后兼容。

## Consequences
正面：
- 第三方无需安装 Selo 即可用 cosign/slsa-verifier 验证。
- 提供承载"角色分离"约束的标准格式（缓解"审计方=签名方"，见 ADR-007）。
- 为 Rekor/TSA 衔接（ADR-005/006）提供标准载体。

负面：
- 需维护 predicate 的 JSON Schema 与版本演进策略。
- 需新增 `change_digest`/`diff_digest` 字段以绑定 subject。
- 引入 `in-toto-golang v0.11.0`（或手写 JSON）。

## Related ADRs
ADR-005（时间戳）、ADR-006（Rekor）、ADR-007（措辞分级）
