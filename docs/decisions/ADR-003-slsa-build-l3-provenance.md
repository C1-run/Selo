# ADR-003: 采用 SLSA Build Level 3 provenance（slsa-github-generator）

## Status
Proposed (2026-10-07)

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
