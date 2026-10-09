# ADR-008: 把签名密钥移出被审计 agent 的信任域

## Status
Proposed (2026-10-09) — Phase 1 & 2 implemented

## Background
ADR-004 堵住了"篡改后重签即通过"，ADR-007 把措辞降级为分级表述。但两份 ADR 都没有解决
最根本的一点：**签名方 = 审计方 = 被审计方，处在同一个信任域**。

2026-10-09 的代码级评审证实了这一点，且比预期更严重：

- `internal/runner/runner.go` 之前把**完整** `os.Environ()` 传给 agent 子进程，
  `SELO_SIGNING_KEY` 直接进入被审计进程。该进程可以读走私钥，签出一份通过
  `selo verify --pubkey <owner>` 的收据。
- 即使清理环境变量，**同一 OS 用户**的 agent 仍可 `cat ~/.selo/signing-key`。
  同用户下无法在进程内阻止这种读盘。

结论：只要密钥落在 agent 可读的信任域内，收据就**不是不可抵赖的**——被审计方能伪造审计
证据。ADR-007 的 L3 需要先解决这一点才能宣称。

## Decision
分阶段把密钥移出 agent 信任域，措辞随能力升级（沿用 ADR-007 阶梯）：

### Phase 1（已实现，2026-10-09）
- `internal/runner` 从 agent 环境剥离 `SELO_SIGNING_KEY` / `SELO_ALLOW_EPHEMERAL_KEY`
  （回归测试 `TestAgentEnvDropsSigningKey`）。
- SECURITY.md 新增 "Threat model" 小节；README "Why not Selo" 增补同信任域限制。
- 公开文案一律不得使用 "non-repudiable"/"tamper-proof"，改 "signed, tamper-evident"。
- **明确不足**：不阻止同用户读盘。仅用于消除最廉价的泄露路径并诚实披露。

### Phase 2（已实现，2026-10-09）
签名抽象为可插拔 `Signer`（`internal/receipt/signer.go`），`SELO_SIGNER` 选择后端：

- `file`（默认）——行为不变：`SELO_SIGNING_KEY` → `~/.selo/signing-key` → 显式允许时
  ephemeral → fail-closed。
- `keychain`——种子存进 **OS keychain**（macOS 用 `security`，Linux 用 `secret-tool`，
  `internal/receipt/keychain.go`，**不引 CGO**，静态产物不受影响）。CLI：
  `selo keys generate --keychain`、`selo keys store --keychain [--delete-file]`、
  `selo keys pub --keychain`。
- `command`——把签名委托给**外部程序**：Selo 把待签字节写 stdin，从 stdout 读回 base64
  签名（`SELO_SIGNER_COMMAND` + `SELO_SIGNER_PUBKEY`）。私钥**永不进入 Selo**。这是
  KMS / HSM / ssh-agent / 独立签名用户 / CI 签名的通用逃生口。

收据新增**签名字段** `key_source`（`env|file|keychain|command|ephemeral`），`selo verify`
会显示它。

**诚实边界**：`keychain` 只是把明文种子从磁盘挪进钥匙串，**同一用户仍可读**（首次访问
可能有系统授权提示，但脚本可被授权）——它抬高门槛，不等于隔离。**只有 `command` 后端，
且其私钥不可导出（HSM/KMS/agent）时，才真正把签名方移出 agent 信任域。** 因此对外措辞
仍停留在 L2（可验证的 attestation），直到有人以 `command`+不可导出密钥部署。

### Phase 3（合规选项）
- **独立签名用户/进程**：agent 以 `selo-agent` 运行，签名方以 `selo-signer` 运行，密钥
  0600 归签名方所有。
- 或 **只在 CI 签**：Selo 产出未签名 transcript，由从不运行 agent 的 CI job 签名。

## Consequences
正面：
- 让"收据可被独立验证、且 agent 无法伪造"成立，这是产品唯一真正的差异化。
- 为 ADR-007 升级到 L3 提供前提。
- Phase 1 零破坏性、可立即发布；Phase 2/3 提供清晰的合规路径。

负面：
- Phase 2 引入平台依赖（keychain/KMS）与配置复杂度。
- Phase 3 需要部署故事（Dockerfile / systemd / CI 模板），超出"本地 runner"定位。
- 在此之前，产品不得使用强断言措辞——短期营销强度下降，换取可信度（与 ADR-007 一致）。

## 升级判据
- L2 "Verifiable attestations"：ADR-001 已实现（DSSE/in-toto 导出）。
- L3 "Non-repudiable"：需 Phase 2 或 Phase 3 **且** ADR-005（时间戳）/ ADR-006（Rekor）。

## Related ADRs
ADR-001（in-toto/DSSE 导出）、ADR-004（持久密钥 fail-closed）、ADR-005（时间戳）、
ADR-006（Rekor）、ADR-007（措辞分级）
