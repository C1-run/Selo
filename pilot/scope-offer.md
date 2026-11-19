# Selo Design-Partner Pilot — Standard Written Scope (offer v1.0)

Date: 2026-11-19 · Provider: C1 (c1.run, team@c1.run) · Product: Selo v0.7.x, github.com/C1-run/Selo (Apache-2.0)

Status: early access, manually delivered. This is the standard written offer issued on
request. Client-specific fields (repository, approver, dates, paths, checks) are completed
in a per-client instance of `scope-template.md` together with filled
`privacy.md` / `ip.md` / `acceptance.md`, and all four are accepted in writing by both
sides before any payment.

## What Selo is (context)

Selo is a free, open-source Go CLI that runs AI coding agents in a Git worktree, audits
the resulting diff after the agent finishes (forbidden files, scope allowlists, secrets,
forbidden claims, patch/round limits, test integrity, required commands), and produces an
Ed25519-signed receipt that a third party can verify with `selo verify <receipt>`. It is a
post-run audit: not real-time interception, not a sandbox, not SAST.

## Scope (what we do)

- One client-nominated repository, for 14 calendar days from kickoff.
- Up to 2 agent-authored changes, run by the client's own supervised agent through Selo
  inside the client's normal workflow.
- Hands-on setup: we configure policy rules and required commands, and wire receipt
  verification into the client's existing CI/review process, working with the client's team.
- One named client-side technical approver reviews at least one receipt as part of a real
  acceptance decision (procedure in `acceptance-template.md`).
- We document the run: setup hours, issues found, and what the approver checked.
- C1 writes no feature code and does not operate the client's agent. The agency runs and
  supervises its own coding agent; C1 assists Selo setup and evidence review only.

## Deliverables

1. A configured policy (`config/selo.yaml`) for the client's repository.
2. Signed receipts for up to 2 agent changes, verified and rendered where the team reviews
   (PR comment / GitHub Actions job summary).
3. A written run summary: what was enforced, what the approver checked, friction found.
4. A handoff note so the client's team keeps running Selo without C1.

## Payment

- USD 500, one time, prepaid: invoiced only after this scope and the privacy/IP/acceptance
  terms are signed by both sides, and payment lands before work starts. No other fees
  during the pilot.
- Cancel before kickoff: full refund.
- If no eligible agent-authored change runs within 14 days: full refund and one free
  re-run after re-scope.

## Exclusions (not included, not promised)

- Any hosted service: scheduled or periodic monitoring, long-term receipt retention
  (Vault), alerting, or policy crosswalk features.
- Real-time interception or sandboxing of the agent process; Selo is post-run by design.
- Vulnerability scanning or SAST; certification or compliance attestations.
- Windows support (Linux/macOS binaries today); changes to the Selo codebase beyond
  configuration.
- Client-controlled signing identity by default: the receipt key is generated fresh in the
  run sandbox and stays operator-side, so receipts are operator-signed, tamper-evident
  evidence, not client-identity attestation (ADR-008 key separation is outside this pilot).

## Effort cap and scope changes

- C1 delivery effort is capped at six hours (setup, signer, diff review, calls, support).
- Anything beyond the capped scope (more repositories, more agent changes, extra
  integrations) is quoted separately in writing before any work starts, recorded with
  `change-request-template.md`.

## Process exhibits

- Before work: a one-page recap (repository, changes, exact commands, reviewer, price,
  no-change outcome) is emailed for confirmation, plus written code-owner authorization
  for agent, Selo and C1 access. Both attach to the scope as exhibits.

## Sign-off

| | |
|---|---|
| Client name / signature / date | _completed in the per-client instance_ |
| C1 (team@c1.run) / date | _completed in the per-client instance_ |
