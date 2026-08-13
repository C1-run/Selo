# C1 Forge Phase 0.8.2 — Test Integrity Gate

## Goal

Prevent agents (C1 Loop, OpenCode) from cheating by deleting or weakening
tests to make them pass. The Test Integrity Gate is a deterministic,
static-analysis-based gate that detects test deletion, test file modification,
test command changes, skip marker injection, and assertion removal — without
running the tests.

## Implementation

### New Package: `internal/testintegrity/`

Self-contained Go package with no external dependencies beyond stdlib.

#### `CaptureInventory(workDir string, testCommands []string) *TestInventory`

Scans a worktree directory at a point in time and returns:
- List of test file paths (relative to worktree root)
- Count of test files found
- Copy of test commands from task config

Test file patterns detected:
- `*_test.go`
- `*.test.ts`, `*.test.tsx`
- `*.spec.ts`, `*.spec.tsx`
- `test_*.py`, `*_test.py`
- `tests/**`, `__tests__/**`
- Paths containing `/tests/` or `/__tests__/`

#### `Analyze(before, after *TestInventory, diff string, allowTestModifications bool) *TestIntegrityResult`

Compares before/after inventories and the runner's diff to detect:

1. **Deleted test files**: files present in `before` but missing in `after`
2. **Modified test files**: files present in both but changed in the diff
3. **Test command changes**: commands added or removed between before/after
4. **Skip markers added** in diff-added lines: `.skip`, `xit(`, `describe.skip(`,
   `test.skip(`, `it.skip(`, `xdescribe`, `xtest`, `#[ignore]`
5. **Assertion-like strings removed** in diff-removed lines: `expect(`, `assert`,
   `require.`, `t.Fatal`, `t.Errorf`, `should`

#### Detection Regexes

- Skip markers: `(\.skip|xit\(|describe\.skip\(|test\.skip\(|it\.skip\(|xdescribe|xtest|#\[ignore\])`
- Assertion-like: `\b(expect\(|assert|require\.|t\.Fatal\b|t\.Errorf\b|should\.)`

#### Verdict Logic

| Violations | `allowTestModifications` | `Passed` | `RecommendedVerdict` |
|------------|--------------------------|----------|----------------------|
| 0          | any                      | true     | (empty)              |
| >0         | false                    | false    | NEEDS_HUMAN          |
| >0         | true                     | true     | NEEDS_HUMAN          |

When `allowTestModifications` is true, the gate "passes" (Passed=true) because
the task explicitly opted into modifications, but the recommended verdict is
always NEEDS_HUMAN — human review is always required when test inventory changes.

### Changes to Existing Code

#### `cmd/c1-forged/main.go`

- Line 202: Parse `allow_test_modifications` from task config (9th return value)
- Lines 255, 367: Capture test inventory before and after runner execution
- Lines 368-380: Run `testintegrity.Analyze()`, write `test_integrity.json` artifact
- Lines 452-467: Apply test integrity gate verdict override (NEEDS_HUMAN on violation)
- Lines 505-511: Populate receipt fields for test integrity
- Lines 527-536: Pass test integrity fields to `WriteReviewMD`

#### `internal/receipt/receipt.go`

New fields in `ForgeReceipt`:
- `test_integrity_passed` — bool
- `test_integrity_result_path` — string
- `tests_removed` — []string
- `tests_modified` — []string
- `test_commands_changed` — []string
- `test_inventory_before_count` — int
- `test_inventory_after_count` — int

Updated `WriteReviewMD` with "## Test Integrity Gate" section.

#### `internal/governor/governor.go`

`ParseTaskConfig` now returns `allowTestModifications` as 9th return value (bool).

### Default Policy

- Test modifications: NOT allowed
- Test deletion: NOT allowed
- Command changes: NOT allowed
- Skip markers: NOT allowed
- When test integrity fails and task doesn't opt in → verdict overridden to NEEDS_HUMAN

## Test Coverage

### Unit Tests (11 tests in `internal/testintegrity/`)

| Test | What it verifies |
|------|-----------------|
| `TestDetectDeletedTestFile` | Removed test file is flagged |
| `TestDetectModifiedTestFile` | Modified test file in diff is flagged |
| `TestDetectPackageJsonTestScriptChange` | Changed test commands are flagged |
| `TestDetectSkipMarkerAdded` | `.skip` markers in diff additions are flagged |
| `TestDetectSkipMarkerRustIgnore` | `#[ignore]` markers are flagged |
| `TestDetectAssertionRemoved` | Assertion-like strings in diff removals are flagged |
| `TestAllowTestModificationWhenTaskAllows` | `allowTestMods=true` still flags but passes |
| `TestInventoryCountShrinkFails` | Test file count decrease is detected |
| `TestNoTestChangesPasses` | No changes = no violation |
| `TestCaptureInventoryFindsTestFiles` | Inventory correctly detects test file patterns |
| `TestIsTestFile` | Edge cases for test file detection |

### E2E Tests (4+1 tests in `cmd/c1-forged/main_test.go`)

| Test | What it verifies |
|------|-----------------|
| `TestDaemonTestIntegrityDetectsTestDeletion` | Fixture agent deleting a test yields NEEDS_HUMAN |
| `TestDaemonTestIntegrityArtifactWritten` | `test_integrity.json` artifact is created |
| `TestDaemonTestIntegrityNoChangePasses` | NOOP task with no changes still passes |
| `TestDaemonTestIntegrityAllowTestModsStillFlagsForReview` | `allowTestMods=true` still flags for review |
| `TestOpenCodeFailingTestDeletionDowngrade` (gated) | OpenCode E2E - test integrity gate catches deletion |

## Design Decisions

1. **Static analysis only**: The gate does not re-run tests. It compares
   test file inventory and analyzes the diff for test-affecting changes.

2. **No false positives for non-test files**: Only files matching test
   file patterns are checked. Documentation changes, config changes, etc.
   are ignored.

3. **Severity-based override**: The gate only overrides the verdict if its
   severity exceeds the current final verdict severity. This preserves
   FAILED_SAFETY, FAILED_TIMEOUT, etc. when they are more severe.

4. **Artifact writing**: `test_integrity.json` is always written, even on
   pass, for audit trail and human review.

5. **Empty recommended verdict**: When the gate passes with no violations,
   `RecommendedVerdict` is empty string `""`, which has severity 0 and
   does not trigger any override.

## Files Changed

| File | Change |
|------|--------|
| `internal/testintegrity/testintegrity.go` | NEW: CaptureInventory, Analyze, TestIntegrityResult |
| `internal/testintegrity/testintegrity_test.go` | NEW: 11 unit tests |
| `cmd/c1-forged/main.go` | ADD: test inventory capture, Analyze call, artifact writing, verdict override, receipt fields |
| `internal/receipt/receipt.go` | ADD: 7 test integrity fields to ForgeReceipt, WriteReviewMD section |
| `internal/governor/governor.go` | ADD: allow_test_modifications parsing in ParseTaskConfig |
| `scripts/fixture_agent.sh` | ADD: DELETE_TEST goal for E2E negative control |
| `cmd/c1-forged/main_test.go` | ADD: 4 E2E tests + 1 gated OpenCode E2E test |
| `docs/C1_FORGE_PHASE0_8_2_TEST_INTEGRITY_GATE.md` | NEW: this file |

## Verdict

```
PASS_C1_FORGE_PHASE0_8_2_TEST_INTEGRITY_GATE
```
