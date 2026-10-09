package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/C1-run/selo/internal/receipt"
	"github.com/C1-run/selo/internal/runner"
	"github.com/C1-run/selo/internal/testintegrity"
	"github.com/spf13/cobra"
)

var selftestCmd = &cobra.Command{
	Use:   "selftest",
	Short: "Prove the safety controls still fire (planted violations must be caught)",
	Long: `selftest plants a known secret, a forbidden-file edit, a forbidden claim, and a
removed test in a throwaway directory, then asserts every control rejects it. It
also confirms signing fails closed with no key present.

It exists because a detector can regress to "always clean" and no test would
notice — that is exactly how the v0.4.x secret scan shipped, on macOS, matching
nothing. Run it in CI so a silent failure of a control cannot ship.`,
	RunE: runSelftest,
}

type selftestCase struct {
	name string
	ok   bool
	note string
}

func runSelftest(cmd *cobra.Command, args []string) error {
	tmp, err := os.MkdirTemp("", "selo-selftest-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	cases := []selftestCase{
		selftestSecretScan(tmp),
		selftestForbiddenFile(),
		selftestForbiddenClaim(tmp),
		selftestTestIntegrity(),
		selftestSigningFailClosed(tmp),
	}

	failed := 0
	for _, c := range cases {
		status := "PASS"
		if !c.ok {
			status = "FAIL"
			failed++
		}
		fmt.Printf("[%s] %s — %s\n", status, c.name, c.note)
	}
	if failed > 0 {
		return fmt.Errorf("selftest: %d/%d controls failed", failed, len(cases))
	}
	fmt.Printf("selftest: all %d controls fired\n", len(cases))
	return nil
}

// selftestSecretScan plants a syntactically valid AWS access-key id and asserts
// the secret scan catches it. This is the control that failed open on macOS.
func selftestSecretScan(dir string) selftestCase {
	c := selftestCase{name: "secret scan rejects a planted AWS key"}
	rel := "creds.go"
	body := "package main\n\nvar key = \"AKIAIOSFODNN7EXAMPLE\"\n"
	if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0644); err != nil {
		c.note = err.Error()
		return c
	}
	hits, err := runner.RunSecretScanDiffScoped(dir, []string{rel})
	if err != nil {
		c.note = err.Error()
		return c
	}
	c.ok = len(hits) > 0
	if c.ok {
		c.note = fmt.Sprintf("caught %d hit", len(hits))
	} else {
		c.note = "planted key was NOT detected"
	}
	return c
}

// selftestForbiddenFile asserts a change to a protected path is flagged.
func selftestForbiddenFile() selftestCase {
	c := selftestCase{name: "forbidden-file check rejects a protected path"}
	diff := "diff --git a/config/secrets.yaml b/config/secrets.yaml\n" +
		"index abc..def 100644\n" +
		"--- a/config/secrets.yaml\n" +
		"+++ b/config/secrets.yaml\n" +
		"@@ -1 +1 @@\n" +
		"-a\n" +
		"+b\n"
	bad, msg := runner.CheckForbiddenFileEdit(diff, "", nil, []string{"config/secrets.yaml"})
	c.ok = bad
	if c.ok {
		c.note = msg
	} else {
		c.note = "forbidden edit was NOT flagged"
	}
	return c
}

// selftestForbiddenClaim plants an overclaim and asserts the diff-scoped scan
// finds it (including the leetspeak/zero-width folding path).
func selftestForbiddenClaim(dir string) selftestCase {
	c := selftestCase{name: "forbidden-claim scan rejects an overclaim"}
	rel := "NOTES.md"
	body := "Status: PR0DUCT1ON_READY\n"
	if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0644); err != nil {
		c.note = err.Error()
		return c
	}
	hits, err := runner.RunForbiddenClaimsScanDiffScoped(dir, []string{"PRODUCTION_READY"}, []string{rel})
	if err != nil {
		c.note = err.Error()
		return c
	}
	c.ok = len(hits) > 0
	if c.ok {
		c.note = fmt.Sprintf("caught %d hit", len(hits))
	} else {
		c.note = "overclaim was NOT detected"
	}
	return c
}

// selftestTestIntegrity asserts a removed test file fails the gate.
func selftestTestIntegrity() selftestCase {
	c := selftestCase{name: "test-integrity gate rejects a removed test"}
	before := &testintegrity.TestInventory{Files: []string{"foo_test.go"}, Count: 1}
	after := &testintegrity.TestInventory{Files: nil, Count: 0}
	res := testintegrity.Analyze(before, after, "", false)
	c.ok = !res.Passed && len(res.TestsRemoved) == 1
	if c.ok {
		c.note = "removed test rejected"
	} else {
		c.note = "removed test was NOT rejected"
	}
	return c
}

// selftestSigningFailClosed asserts that with no key at all, signing is
// refused rather than silently producing an unattributable receipt.
func selftestSigningFailClosed(dir string) selftestCase {
	c := selftestCase{name: "signing fails closed with no key"}
	emptyHome := filepath.Join(dir, "empty-home")
	if err := os.MkdirAll(emptyHome, 0700); err != nil {
		c.note = err.Error()
		return c
	}
	restore := swapEnv(map[string]*string{
		"SELO_SIGNING_KEY":         nil,
		"SELO_ALLOW_EPHEMERAL_KEY": nil,
		"SELO_SIGNER":              nil,
		"HOME":                     &emptyHome,
	})
	defer restore()

	r := &receipt.ForgeReceipt{ReceiptID: "selftest", Verdict: receipt.VerdictSuccess}
	if _, err := receipt.SignReceipt(r); err != nil {
		c.ok = true
		c.note = "refused to sign without a key"
	} else {
		c.note = "signing succeeded with no key — fail-closed is broken"
	}
	return c
}

// swapEnv sets each key to the given value (nil unsets it) and returns a
// function that restores the original environment.
func swapEnv(vals map[string]*string) func() {
	orig := make(map[string]*string, len(vals))
	for k, v := range vals {
		if old, ok := os.LookupEnv(k); ok {
			o := old
			orig[k] = &o
		} else {
			orig[k] = nil
		}
		if v == nil {
			os.Unsetenv(k)
		} else {
			os.Setenv(k, *v)
		}
	}
	return func() {
		for k, v := range orig {
			if v == nil {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, *v)
			}
		}
	}
}
