# ADR-006: 收据锚定 Sigstore Rekor 透明日志

## Status
Accepted (2026-10-09) — implemented

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

## Implementation（2026-10-09）

已实现。关键文件：`internal/receipt/rekor.go`；CLI：`selo run --rekor/--rekor-soft`、`selo receipt log`、`selo verify --rekor-pubkey/--require-rekor`。

- **不引入 `sigstore-go`**（与原文的差异）。直接用 Rekor 的 REST API（stdlib `net/http`）上传 `/api/v1/log/entries`，从而**不必**把项目 Go 升到 1.23+、也**不必**背上重依赖，`CGO_ENABLED=0` 静态二进制不受影响。原文列为负面项的「重依赖 + Go 版本升级」因此消失。
- **上传形态**：`dsse` entry，其 DSSE payload 就是**收据哈希**，由收据的签名者签名（`ResolveSigner`），`verifiers` 为 base64(PEM) 公钥。Rekor 会先校验信封签名再接受。
- **隐私（默认裁剪）**：Rekor 对 `dsse` entry 只保存 **envelopeHash / payloadHash**（不含 payload 明文）。因此任务名、文件路径、收据内容都不出本机——无需私有 Rekor 即可满足本 ADR 的隐私要求。
- **存储**：收据新增 `transparency` 字段（`log_url` / `uuid` / `log_id` / `log_index` / `integrated_time` / `payload_hash` / `envelope_hash` / `body`(base64 canonical entry) / `signed_entry_timestamp` / `inclusion_proof`(checkpoint+hashes+root+size) / `status` / `reason`）。与 `timestamp` 一样在签名后产生，**不参与** canonical JSON。
- **离线验证**：`VerifyTransparency` 重算 leaf hash `sha256(0x00‖body)`，按 RFC 6962 走审计路径到 root，比对 checkpoint 的 root 与 treeSize，再以 `--rekor-pubkey` 校验 checkpoint 的 note 签名（ECDSA/SHA-256 或 Ed25519）。未给 `--rekor-pubkey` 时报 `UNVERIFIED`。`--require-rekor` 将任何非 OK 判为失败。
- **失败语义**：默认不可达即非零退出；`--rekor-soft` 降级为 `status: absent` 并记录原因。
- **验证程度**：Merkle 包含证明与 checkpoint 签名逻辑已用**线上真实 Rekor 数据**（只读拉取 `rekor.sigstore.dev` 的 entry + log public key）核对通过；上传路径已对 **Sigstore staging**（`rekor.sigstage.dev`，专供测试）做真实写入验证（HTTP 201），并在端到端流程中产出 `Transparency: OK`。
