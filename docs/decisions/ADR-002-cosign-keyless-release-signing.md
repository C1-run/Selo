# ADR-002: 使用 Sigstore keyless（cosign + OIDC）签名发布产物

## Status
Proposed (2026-10-07)

## Background
`release.yml` 当前只把二进制挂到 Release，**无 checksums、无签名**；`install.sh` 下载后零校验（`install.sh:38-46`）。一个售卖"可验证性"的工具用无校验方式分发自身，自相矛盾，且供应链可被投毒。需要一套无需长期密钥管理、可被公众独立验证的发布签名方案。

## Decision
在 release workflow 中：
- 生成确定性 `checksums.txt`（sha256，排序）。
- 用 **cosign v3.1.2 keyless（OIDC）** 签名 `checksums.txt`，产出 Sigstore bundle `checksums.txt.sigstore.json`。
- 用 `sigstore/cosign-installer@v3`，`permissions: id-token: write`。
- `install.sh` 下载后**必须**校验 checksum + cosign 签名，并**钉死** `--certificate-identity-regexp` 与 `--certificate-oidc-issuer`；失败即 `exit 1`。
- 允许 `SELO_SKIP_VERIFY=1` 显式跳过，但默认关闭且大声警告。

## Consequences
正面：
- 无需长期签名密钥（keyless），身份绑定到 GitHub 工作流 OIDC，签名可公开验证。
- 与 Selo"可验证"叙事一致（dogfooding）。
- 防供应链投毒：篡改任一字节即校验失败。

负面：
- 依赖 Sigstore 公共基础设施（Fulcio/Rekor/TUF）与网络。
- install.sh 增加对 cosign 的依赖（无 cosign 时须明确失败而非静默跳过）。
- keyless 签名短期证书需依赖 Rekor 透明度，需接受其可用性。

## Related ADRs
ADR-003（SLSA provenance）、ADR-006（Rekor）
