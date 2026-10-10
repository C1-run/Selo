# ADR-002: 使用 Sigstore keyless（cosign + OIDC）签名发布产物

## Status
Accepted (2026-10-09) — implemented

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

## Implementation（2026-10-09）

已实现。关键文件：`.github/workflows/release.yml`、`install.sh`；测试：`scripts/test-install.sh`。

- **发布流水线重构**：原 `release.yml` 用 matrix 分别构建 4 个二进制并各自上传，**没有任何一个时刻所有产物同时存在**——这既无法生成统一的 `checksums.txt`，也无法给 SLSA 生成器提供 subjects。改为**单 build job** 交叉编译全部 4 个目标（`CGO_ENABLED=0`，Go 交叉编译无需并行即可完成），产出确定性的 `checksums.txt`。同时把 workflow 级权限收紧为 `contents: read`，各 job 按需授权。
- **确定性 checksum**：`sha256sum selo-* | LC_ALL=C sort -k2 > checksums.txt`。排序是必须的——同样输入必须产出逐字节相同的文件，否则签名不可复现。已在本地模拟验证：格式为 `<64hex>  <name>`（与 `sha256sum` 一致）、重复生成逐字节相同、base64 往返无损。
- **签名**：`cosign sign-blob --yes --bundle checksums.txt.sigstore.json checksums.txt`，经 `sigstore/cosign-installer@v3`（`cosign-release: v3.1.2`），job 授权 `id-token: write`。keyless 故无常驻密钥，Fulcio 证书绑定到 workflow 的 OIDC 身份。
- **install.sh 改为 fail-closed**：下载二进制 + `checksums.txt` + bundle 三件套；先校验二进制 sha256，再用 `cosign verify-blob --bundle ... --certificate-identity-regexp ... --certificate-oidc-issuer ...` 校验 `checksums.txt`。**任一环节失败即 `exit 1` 且不安装**；缺 cosign 时明确报错并给出安装指引，而非静默跳过。`SELO_SKIP_VERIFY=1` 为显式逃生口，默认关闭且大声告警。另外：目标目录可写时不再无谓地 `sudo`；API 取版本号改用 `grep -m1`，避免 `head` 提前关管道触发 SIGPIPE 被 `pipefail` 误判为失败。
- **关键发现（原提案未提及）**：cosign 的 keyless 校验中 `--certificate-identity(-regexp)` 与 `--certificate-oidc-issuer` **两者都是强制的**（官方文档：Either ... must be set for keyless flows）。即"同时钉死身份与签发方"不只是最佳实践，而是命令能工作的前提。默认钉死为 `^https://github\.com/C1-run/selo/\.github/workflows/release\.yml@refs/tags/v.*$` + `https://token.actions.githubusercontent.com`。已验证该正则**接受**本仓库 release workflow 的 tag 身份，并**拒绝**外部仓库、本仓库其它 workflow（如 `ci.yml`）以及分支 ref。
- **测试边界（诚实说明）**：`scripts/test-install.sh` 以 `file://` 供给伪造产物 + cosign shim，**无网络**地覆盖 12 项断言：happy path、checksum 被篡改、二进制被篡改、缺 cosign、cosign 校验失败、`SELO_SKIP_VERIFY` 逃生口（含告警）、缺 `checksums.txt`、缺 bundle、产物未列入 checksums。它锁定的是**控制流**；**真实的 Fulcio/Rekor 校验无法在本地验证**，只有真正打 tag 跑一次 release 才能端到端确认。已接入 `ci.yml`。
- **遗留（owner 侧）**：第三方 action（`actions/*`、`softprops/action-gh-release`、`sigstore/cosign-installer`）目前按主版本 tag 引用而非 commit SHA 钉死。SHA 钉死需联网解析且需依赖自动更新维护，故留作后续硬化项；`slsa-github-generator` 则**必须**用 tag（见 ADR-003）。

## 后续修订（2026-10-10）：改用直接下载 + 摘要钉死的 cosign

**触发**：v0.6.0 是本 workflow 落地（`65cf77c`）之后的**第一个 tag**。release job 在
「Install cosign」一步失败，exit 22，于是「Sign checksums」「Publish release」全部被
跳过——release 页上只剩 provenance job 单独上传的 `multiple.intoto.jsonl`，四个二进制、
`checksums.txt`、签名、release notes 一个都没上去。

**根因（已取证）**：`sigstore/cosign-installer@v3` 无法安装任何 cosign **v3.x**。它在
下载二进制之后，还会再取 legacy detached signature 并用 `release-cosign.pub` 校验：

    curl -fsLO https://github.com/sigstore/cosign/releases/download/v3.1.2/cosign-linux-amd64.sig

而 cosign v3.x 已经不再发布 `.sig` 资产（改为 `.sigstore.json` bundle）。实测：

    .../v3.1.2/cosign-linux-amd64.sig  -> 404
    .../v2.6.5/cosign-linux-amd64.sig  -> 200

`curl -f` 遇 404 的退出码正是 **22**，与 CI 观测到的完全一致。也就是说这个 pin
从写下那天起就是坏的，只是因为没有 tag 触发过 release workflow 而无人发现。

**修订**：不再使用该 action，改为直接下载 + **在本仓库内钉死 SHA-256**：

    COSIGN_SHA256=f7622ed3cf22e55e1ae6377c080979ff77a22da9981c11df222a2e444991e7cf
    curl -fsSL --retry 3 --retry-all-errors -o cosign .../v3.1.2/cosign-linux-amd64
    echo "${COSIGN_SHA256}  cosign" | sha256sum -c -

该摘要取自 cosign 自己发布的 `cosign_checksums.txt`，并已通过实际下载 + 哈希复核。
效果上这是**减少**了一处第三方信任面：版本→摘要的绑定现在位于我们自己可 review 的
树里，而不是在别人的 action 内部。

**教训（与 ADR-003 同类）**：一条从未被触发过的发布路径，等价于不存在。本 workflow
从 `65cf77c` 到 v0.6.0 之间没有任何 tag，所以「签名发布」一直是纸面能力。这与 darwin
二进制连续三个版本不可加载是同一个失效模式：**没有在真实路径上跑过的东西，不能算
完成。**
