# ADR-003: 采用 SLSA Build Level 3 provenance（slsa-github-generator）

## Status
Accepted (2026-10-09) — implemented

## Background
发布产物即使有 checksum 与签名，也只证明"这个二进制没被改"，不证明"它确实由本仓库、本工作流、本 commit 构建而来"。供应链攻击常发生在构建环节（构建系统被污染）。需要**构建来源证明（provenance）**，且应达到 SLSA Build L3（防篡改、可独立验证的构建来源）。

## Decision
在 release workflow 中引入 **slsa-framework/slsa-github-generator v2.1.0** 的 reusable workflow：
- `uses: slsa-framework/slsa-github-generator/.github/workflows/generator_generic_slsa3.yml@v2.1.0`
- 输入 `base64-subjects` = `sha256sum selo-* | base64`，`upload-assets: true`。
- 产出 `multiple.intoto.jsonl`（in-toto Statement，`predicateType = https://slsa.dev/provenance/v1`），随 Release 发布。
- 生产环境将 reusable workflow 与 installer **按 commit SHA 固定**（防 tag 漂移）。

## Consequences
正面：
- 达成 SLSA Build L3 的"不可伪造来源证明"，第三方可用 `slsa-verifier` 验证。
- 与 ADR-002 的 cosign 签名互补：签名答"谁签的"，provenance 答"怎么构建的"。
- 与方案 03 的 in-toto 生态一致。

负面：
- 依赖 GitHub Actions 与 slsa-github-generator 的可用性。
- 增加 CI 复杂度与构建时长。
- L3 依赖托管 runner 的信任假设（GitHub 侧）。

## Related ADRs
ADR-002（cosign 签名）、ADR-001（in-toto 格式）

## Implementation（2026-10-09）

已实现。关键文件：`.github/workflows/release.yml` 的 `provenance` job。

- **接入方式**：`uses: slsa-framework/slsa-github-generator/.github/workflows/generator_generic_slsa3.yml@v2.1.0`，`with: { base64-subjects, upload-assets: true }`，调用方 job 授权 `actions: read` + `id-token: write` + `contents: write`。已核实 `v2.1.0` 为真实 tag（该 tag 下的 workflow 文件可取得，且其内部 `uses:` 全部钉在 `@v2.1.0` 而非 `@main`）。产出 `multiple.intoto.jsonl`（`predicateType = https://slsa.dev/provenance/v1`），随 Release 发布。
- **一处复用**：`base64-subjects` 正是 `base64(checksums.txt)`。因此 ADR-002 中被 cosign 签名的那个 `checksums.txt`，同一份内容直接充当 SLSA 的 subjects——**一个产物同时支撑两套保证**，无需重复计算或额外文件。已本地验证 `base64` 往返无损、格式符合生成器要求的 `SHA256 NAME\n`。
- **原提案中的内在张力（已裁决）**：ADR-003 原文要求"生产环境按 commit SHA 固定 reusable workflow（防 tag 漂移）"。但生成器文档明确：`compile-generator: false` 时构建器二进制是**从该 ref 的 release 下载**的——"This must be a tag reference"。故 **SHA 钉死会强制 `compile-generator: true`**（+约 2 分钟构建）。经权衡，采用**完整版本 tag `@v2.1.0`**（非浮动主版本号）：这是 SLSA 官方文档自身的推荐做法，且生成器会对 builder 身份签名，漂移风险由验证侧兜底。代价与理由记录于此，便于日后若需 SHA 钉死时直接切换并接受 +2min。
- **备选方案（已评估并否决）**：GitHub 原生的 `actions/attest-build-provenance`。其签名与构建发生在**同一 job**，属于 SLSA **Build L2** 而非 L3；本 ADR 明确要求 L3（隔离构建 + 不可伪造来源），故不采用。若日后仅需 L2 的便利性，可再评估。
- **第三方验证**：`slsa-verifier verify-artifact <file> --provenance-path multiple.intoto.jsonl --source-uri github.com/C1-run/selo`。
- **测试边界（诚实说明）**：provenance 的生成与验证**只能在 GitHub Actions 上完成**，本地无法执行（无托管 runner、无 OIDC）。本项**未**做本地端到端验证——已核对的仅是：tag 真实存在、输入名与类型正确、调用方权限齐备、`base64-subjects` 格式正确。**首次打 tag 时必须人工确认该 job 成功**。
- **遗留（owner 侧）**：与 ADR-002 相同，需在真实 release 后确认 provenance 已上传至 Release 且 `slsa-verifier` 校验通过。
