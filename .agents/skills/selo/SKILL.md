---
name: selo
description: Submit coding tasks to the local Selo daemon and surface the signed receipt verdict before claiming any task done. Use whenever the user asks to run a task through Selo, wants an audited or verified agent run, mentions receipts, selo run, or the daemon, or asks whether a change passed the safety checks — even if they just say "run it through selo" or "did the checks pass".
---

# Selo: audited agent tasks with signed receipts

Selo wraps a coding task in a git worktree, audits what the agent changed
afterwards (forbidden files, scope violations, secrets, forbidden claims,
test tampering), and writes an Ed25519-signed receipt with a verdict.

The core rule: **you never self-certify a Selo task.** The receipt is the
claim of record. Your prose summary is not evidence — the card is.

## 1. Make sure Selo is runnable

```bash
test -x ./selo || go build -o selo ./cmd/selo/   # from the selo repo checkout
test -f config/selo.yaml || ./selo init
```

If there is no Selo checkout or config in this project, tell the user Selo
is not initialized here and stop — do not fake the workflow.

## 2. Run the task

One-shot, blocking, always produces a receipt either way:

```bash
./selo run "<the task goal>"
```

The command exits non-zero on safety failures and timeouts. That is the
product working, not a malfunction — capture the receipt instead of retrying
blindly.

## 3. Show the verdict before summarizing

Find the newest receipt and render the card into the conversation:

```bash
./selo receipt list                 # find the receipt id
./selo receipt show <id> --format github   # or plain --format markdown/text
```

Always include the card (or its verdict and integrity lines) in your reply.
Never paraphrase the verdict from memory of the run — read it from the
receipt. The card includes the integrity state (hash, signature); an
INVALID receipt means the file was altered or is malformed: say so plainly
and stop, do not treat the task as done.

## 4. Be honest about what Selo did

- The audit is **post-execution**: Selo documents and rejects violating
  changes inside a contained worktree, it does not intercept the agent
  mid-run. Do not claim real-time prevention.
- If the verdict is a rejection, report it as a rejection. Rewriting a
  safety failure as a success in prose is exactly the false-claim behavior
  Selo's forbidden-claims scan exists to catch — the receipt will still
  show the truth.

## 5. Inspecting past work

```bash
./selo receipt list                 # archive overview, newest last
./selo verify receipts/<id>.json    # third-party integrity check
```

## Example flow

```
User: run a task through selo: add a README section about receipts
You:  (build/init if needed)
      $ ./selo run "add a README section about receipts"
      $ ./selo receipt show r-abc123 --format github
      → paste the card, then summarize what the receipt says
```
