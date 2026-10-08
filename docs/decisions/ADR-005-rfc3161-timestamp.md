# ADR-005: 收据附加 RFC 3161 可信时间戳

## Status
Proposed (2026-10-07)

## Background
当前收据的时间字段（`started_at`/`finished_at`/`anchored_at`）全部由 Selo 自己写入，属"自报时间"，可被事后倒填或补造。要支撑"不可抵赖"，必须证明"签名发生在某个**外部可验证**的时间点"。这是"防篡改"（tamper-evidence）与"不可抵赖"之间的关键一环。

## Decision
签名后，将收据哈希（或其 DSSE 信封）提交 **RFC 3161 TSA**，取回 timestamp token，随收据保存：
- 收据/ predicate 新增 `timestamp.token`（base64 DER）与 `tsa_url`。
- CLI 增加 `--tsa <URL>`；`selo receipt export` 支持附带时间戳。
- TSA 选型：PoC 用 freetsa.org；生产用付费 TSA（DigiCert/Sectigo）或 **Sigstore TSA**（经 `sigstore-go v1.3.0`）。
- TSA 不可达时的策略：**不得静默产出"无时间戳却声称不可抵赖"的收据**——要么失败（非零退出），要么降级并在收据标注 `timestamp: absent`。

## Consequences
正面：
- 提供外部可验证的签名时点，堵住倒填/补造。
- 成本低、实现轻，可与 ADR-006 互补。
- 独立于 Rekor 也能提供时间保证。

负面：
- 依赖 TSA 服务可用性与限流（免费 TSA 不适合生产 SLA）。
- 需管理/信任 TSA 证书链。
- 增加一次网络往返，签名路径变慢。

## Related ADRs
ADR-004（持久密钥）、ADR-006（Rekor）、ADR-007（措辞分级）
