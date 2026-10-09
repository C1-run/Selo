# ADR-005: 收据附加 RFC 3161 可信时间戳

## Status
Accepted (2026-10-09) — implemented

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

## Implementation（2026-10-09）

已实现。关键文件：`internal/receipt/tsa.go`；CLI：`selo run --tsa/--tsa-soft`、`selo receipt timestamp`、`selo verify --tsa-ca/--require-tsa`。

- **时间戳对象**：收据新增 `timestamp` 字段（`tsa_url` / `token`(base64 DER) / `gen_time` / `serial` / `policy` / `digest` / `status` / `reason`）。它在签名**之后**产生，因此与 git anchor 字段一样**不参与** canonical JSON —— 否则会形成循环签名。
- **绑定**：messageImprint = sha256(canonical JSON) = `receipt_hash`。于是时间戳同时绑定收据内容与（经由 canonical 的）签名，且无法被倒填。
- **失败语义（fail-closed）**：默认 TSA 不可达即非零退出；`--tsa-soft`（或 `SELO_TSA_SOFT=1`）时降级为 `status: absent` 并记录 `reason`，绝不静默省略——满足本 ADR「不得静默产出」的要求。
- **验证分级**：`selo verify` 始终检查 token 结构、CMS 签名（库内 `pkcs7.Verify`）与 messageImprint；给出 `--tsa-ca` 时额外要求 TSA 证书链到受信根、且在**时间戳时刻**具备 timeStamping EKU。未给 `--tsa-ca` 时报 `UNVERIFIED`（仅自洽、未锚定），与「未 pin 的签名者」同一口径。`--require-tsa` 将任何非 OK 判为失败。
- **实现选择（与原提案的差异）**：使用 `github.com/digitorus/timestamp`，pin 到 go1.16 期的 `v0.0.0-20250524132541-c45532741eea`（依赖树仅两个纯 Go 模块、无 CGO）。**未**采用原文提到的 `sigstore-go`——它会强制 Go 1.23+ 并引入重依赖。该 pin 版本不含带信任链的 `Verify()`，故证书链校验由 Selo 用 `crypto/x509` 自行完成。
- **测试**：`internal/receipt/tsa_test.go` 以本地自建 TSA（自签 TSA 证书 + 库的服务端接口）做**无网络**单测，覆盖 imprint 不符、token 被篡改、根不受信、不可达降级等；另以 freetsa.org 真实 TSA 做过端到端验证（`Timestamp: OK`，证书链锚定到真实根）。
