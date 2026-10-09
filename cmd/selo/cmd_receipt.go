package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/C1-run/selo/internal/receipt"
	"github.com/spf13/cobra"
)

var (
	receiptDir          string
	receiptLimit        int
	receiptJSONOut      bool
	receiptFormat       string
	receiptWantAnchor   bool
	receiptPubKey       string
	receiptExportFormat string
	receiptExportOut    string
	receiptTSA          string
	receiptTSASoft      bool
	receiptRekor        string
	receiptRekorSoft    bool
)

var receiptCmd = &cobra.Command{
	Use:   "receipt",
	Short: "List and inspect signed receipts",
	Long: `List receipts in the archive or render one as a decision card.

The card always includes the receipt's integrity state (content hash and
Ed25519 signature), so a tampered receipt cannot be mistaken for a clean one.
Third parties can reproduce the check with: selo verify <receipt.json>`,
}

var receiptListCmd = &cobra.Command{
	Use:   "list",
	Short: "List receipts in the archive (newest last)",
	RunE:  runReceiptList,
}

var receiptShowCmd = &cobra.Command{
	Use:   "show <id-or-path>",
	Short: "Render a receipt as a decision card",
	Args:  cobra.ExactArgs(1),
	RunE:  runReceiptShow,
}

var receiptExportCmd = &cobra.Command{
	Use:   "export <id-or-path>",
	Short: "Export a receipt as a standard in-toto attestation (DSSE)",
	Long: `Export re-signs a receipt as an in-toto Statement v1 wrapped in a DSSE
envelope, so a third party can verify it with standard tooling (cosign,
slsa-verifier, a policy engine) without installing Selo.

The native receipt.json format is unchanged; this is an additional,
interoperable representation. The signature covers the DSSE PAE over the exact
statement bytes — not a re-serialization — so a verifier in any language can
check it. The signing key is resolved like receipt signing (SELO_SIGNING_KEY,
then ~/.selo/signing-key; fail-closed with no key).`,
	Args: cobra.ExactArgs(1),
	RunE: runReceiptExport,
}

var receiptTimestampCmd = &cobra.Command{
	Use:   "timestamp <id-or-path>",
	Short: "Attach an RFC3161 trusted timestamp to an existing receipt (ADR-005)",
	Long: `Attach an RFC3161 trusted timestamp to a receipt that was signed without one.

The timestamp is obtained over the receipt's canonical bytes (its receipt_hash),
so it proves the signed receipt existed at an externally verifiable instant and
cannot be back-dated. Selo refuses to timestamp a receipt whose signature or
content hash does not verify — timestamping a tampered receipt would launder it.
Fail-closed unless --soft is given, which records the absence instead.`,
	Args: cobra.ExactArgs(1),
	RunE: runReceiptTimestamp,
}

var receiptLogCmd = &cobra.Command{
	Use:   "log <id-or-path>",
	Short: "Log an existing receipt in a Sigstore Rekor transparency log (ADR-006)",
	Long: `Log a signed receipt in a Sigstore Rekor transparency log, so the run is
publicly witnessed and cannot be equivocated or quietly withdrawn.

Only the receipt hash is published: Selo submits a DSSE entry whose payload is
the hash, and Rekor stores just the envelope and payload hashes — never the
receipt, task name, or file paths. Like 'receipt timestamp', Selo refuses to log
a receipt whose signature or content hash does not verify. Fail-closed unless
--soft is given, which records the absence instead.`,
	Args: cobra.ExactArgs(1),
	RunE: runReceiptLog,
}

func init() {
	receiptListCmd.Flags().StringVar(&receiptDir, "dir", "", "Base directory (default: auto-detect)")
	receiptListCmd.Flags().IntVar(&receiptLimit, "limit", 20, "Maximum receipts to list")
	receiptListCmd.Flags().BoolVar(&receiptJSONOut, "json", false, "Output as JSON")

	receiptShowCmd.Flags().StringVar(&receiptDir, "dir", "", "Base directory (default: auto-detect)")
	receiptShowCmd.Flags().StringVar(&receiptFormat, "format", "text", "Output format: text, markdown, github, or json")
	receiptShowCmd.Flags().BoolVar(&receiptWantAnchor, "anchor", false, "Also verify the git anchor")
	receiptShowCmd.Flags().StringVar(&receiptPubKey, "pubkey", "", "Pin the signer: a path to a key file, a 64-char hex fingerprint, or an inline base64 public key")

	receiptExportCmd.Flags().StringVar(&receiptDir, "dir", "", "Base directory (default: auto-detect)")
	receiptExportCmd.Flags().StringVar(&receiptExportFormat, "format", "in-toto", "Export format (only \"in-toto\")")
	receiptExportCmd.Flags().StringVar(&receiptExportOut, "out", "", "Write to this file (default: stdout)")

	receiptTimestampCmd.Flags().StringVar(&receiptDir, "dir", "", "Base directory (default: auto-detect)")
	receiptTimestampCmd.Flags().StringVar(&receiptTSA, "tsa", "", "RFC3161 timestamp authority URL (required)")
	receiptTimestampCmd.Flags().BoolVar(&receiptTSASoft, "soft", false, "If the TSA is unreachable, record the timestamp as absent instead of failing")

	receiptLogCmd.Flags().StringVar(&receiptDir, "dir", "", "Base directory (default: auto-detect)")
	receiptLogCmd.Flags().StringVar(&receiptRekor, "rekor", "", "Sigstore Rekor transparency-log URL (required)")
	receiptLogCmd.Flags().BoolVar(&receiptRekorSoft, "soft", false, "If the log is unreachable, record the entry as absent instead of failing")

	receiptCmd.AddCommand(receiptListCmd, receiptShowCmd, receiptExportCmd, receiptTimestampCmd, receiptLogCmd)
}

// runReceiptLog logs an existing signed receipt in a Rekor transparency log.
func runReceiptLog(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(receiptRekor) == "" {
		return fmt.Errorf("--rekor <url> is required")
	}
	baseDir := receiptDir
	if baseDir == "" {
		var err error
		baseDir, err = findBaseDir(globalCfgFile)
		if err != nil {
			return fmt.Errorf("cannot find base dir: %w", err)
		}
	}
	path := resolveReceiptPath(args[0], baseDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read receipt: %w", err)
	}
	var r receipt.ForgeReceipt
	if err := json.Unmarshal(data, &r); err != nil {
		return fmt.Errorf("parse receipt: %w", err)
	}

	// Refuse to log a receipt whose integrity does not hold: a public log entry
	// for a tampered receipt would launder it into looking witnessed.
	if ok, verr := receipt.VerifyReceipt(&r); verr != nil || !ok {
		if verr != nil {
			return fmt.Errorf("refusing to log a receipt whose signature does not verify: %w", verr)
		}
		return fmt.Errorf("refusing to log a receipt whose signature does not verify")
	}
	canonical, err := receipt.CanonicalJSON(&r)
	if err != nil {
		return fmt.Errorf("canonical json: %w", err)
	}
	sum := sha256.Sum256(canonical)
	if r.ReceiptHash != hex.EncodeToString(sum[:]) {
		return fmt.Errorf("refusing to log a receipt whose content hash does not match")
	}

	signer, err := receipt.ResolveSigner()
	if err != nil {
		return err
	}
	tr, err := receipt.UploadToRekor(r.ReceiptHash, signer, receipt.TransparencyOptions{
		LogURL: receiptRekor,
		Soft:   receiptRekorSoft,
	})
	if err != nil {
		return err
	}
	r.Transparency = tr
	out, err := json.MarshalIndent(&r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal receipt: %w", err)
	}
	out = append(out, '\n')
	if err := os.WriteFile(path, out, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if tr.Status == receipt.TransparencyLogged {
		fmt.Printf("Logged %s at %s (index %d, integrated %s)\n", path, tr.LogURL, tr.LogIndex,
			time.Unix(tr.IntegratedTime, 0).UTC().Format(time.RFC3339))
	} else {
		fmt.Printf("Transparency record absent for %s: %s\n", path, tr.Reason)
	}
	return nil
}

// runReceiptTimestamp attaches an RFC3161 timestamp to an existing receipt.
func runReceiptTimestamp(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(receiptTSA) == "" {
		return fmt.Errorf("--tsa <url> is required")
	}
	baseDir := receiptDir
	if baseDir == "" {
		var err error
		baseDir, err = findBaseDir(globalCfgFile)
		if err != nil {
			return fmt.Errorf("cannot find base dir: %w", err)
		}
	}
	path := resolveReceiptPath(args[0], baseDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read receipt: %w", err)
	}
	var r receipt.ForgeReceipt
	if err := json.Unmarshal(data, &r); err != nil {
		return fmt.Errorf("parse receipt: %w", err)
	}

	// Refuse to timestamp a receipt whose integrity does not hold: a timestamp
	// on a tampered receipt would launder it into looking externally attested.
	if ok, verr := receipt.VerifyReceipt(&r); verr != nil || !ok {
		if verr != nil {
			return fmt.Errorf("refusing to timestamp a receipt whose signature does not verify: %w", verr)
		}
		return fmt.Errorf("refusing to timestamp a receipt whose signature does not verify")
	}
	canonical, err := receipt.CanonicalJSON(&r)
	if err != nil {
		return fmt.Errorf("canonical json: %w", err)
	}
	sum := sha256.Sum256(canonical)
	if r.ReceiptHash != hex.EncodeToString(sum[:]) {
		return fmt.Errorf("refusing to timestamp a receipt whose content hash does not match")
	}

	anc, err := receipt.TimestampCanonical(canonical, receipt.TimestampOptions{
		TSAURL: receiptTSA,
		Soft:   receiptTSASoft,
	})
	if err != nil {
		return err
	}
	r.Timestamp = anc
	out, err := json.MarshalIndent(&r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal receipt: %w", err)
	}
	out = append(out, '\n')
	if err := os.WriteFile(path, out, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if anc.Status == receipt.TimestampGranted {
		fmt.Printf("Timestamped %s at %s (%s)\n", path, anc.GenTime.UTC().Format(time.RFC3339), anc.TSAURL)
	} else {
		fmt.Printf("Timestamp absent for %s: %s\n", path, anc.Reason)
	}
	return nil
}

// runReceiptExport writes an in-toto/DSSE attestation for a receipt.
func runReceiptExport(cmd *cobra.Command, args []string) error {
	if receiptExportFormat != "in-toto" {
		return fmt.Errorf("unknown export format %q (only \"in-toto\")", receiptExportFormat)
	}
	baseDir := receiptDir
	if baseDir == "" {
		var err error
		baseDir, err = findBaseDir(globalCfgFile)
		if err != nil {
			return fmt.Errorf("cannot find base dir: %w", err)
		}
	}

	path := resolveReceiptPath(args[0], baseDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read receipt: %w", err)
	}
	var r receipt.ForgeReceipt
	if err := json.Unmarshal(data, &r); err != nil {
		return fmt.Errorf("parse receipt: %w", err)
	}

	env, err := receipt.ExportInToto(&r)
	if err != nil {
		return fmt.Errorf("export in-toto: %w", err)
	}
	out, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	out = append(out, '\n')

	if receiptExportOut != "" {
		if err := os.WriteFile(receiptExportOut, out, 0644); err != nil {
			return fmt.Errorf("write %s: %w", receiptExportOut, err)
		}
		fmt.Printf("Wrote in-toto attestation: %s\n", receiptExportOut)
		return nil
	}
	_, err = os.Stdout.Write(out)
	return err
}

// resolveReceiptPath maps a receipt ID or path to a receipt.json file.
// An ID is looked up in <base>/receipts/<id>.json, then <base>/runs/run-<id>/receipt.json.
func resolveReceiptPath(arg, baseDir string) string {
	if strings.Contains(arg, string(os.PathSeparator)) || strings.HasSuffix(arg, ".json") {
		return arg
	}
	candidates := []string{
		filepath.Join(baseDir, "receipts", arg+".json"),
		filepath.Join(baseDir, "receipts", "c1f-"+arg+".json"),
		filepath.Join(baseDir, "runs", "run-"+arg, "receipt.json"),
		filepath.Join(baseDir, "runs", arg, "receipt.json"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return candidates[0] // will fail downstream with a clear unreadable-receipt error
}

func runReceiptList(cmd *cobra.Command, args []string) error {
	baseDir := receiptDir
	if baseDir == "" {
		var err error
		baseDir, err = findBaseDir(globalCfgFile)
		if err != nil {
			return fmt.Errorf("cannot find base dir: %w", err)
		}
	}

	entries, err := os.ReadDir(filepath.Join(baseDir, "receipts"))
	if err != nil {
		return fmt.Errorf("reading receipts archive in %s: %w", baseDir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) > receiptLimit {
		names = names[len(names)-receiptLimit:]
	}

	type row struct {
		ReceiptID string `json:"receipt_id"`
		TaskID    string `json:"task_id"`
		Verdict   string `json:"verdict"`
		StartedAt string `json:"started_at"`
	}
	rows := make([]row, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(baseDir, "receipts", name))
		if err != nil {
			continue
		}
		var r struct {
			ReceiptID string `json:"receipt_id"`
			TaskID    string `json:"task_id"`
			Verdict   string `json:"verdict"`
			StartedAt string `json:"started_at"`
		}
		if json.Unmarshal(data, &r) != nil {
			continue
		}
		if r.ReceiptID == "" {
			r.ReceiptID = strings.TrimSuffix(name, ".json")
		}
		rows = append(rows, row{r.ReceiptID, r.TaskID, r.Verdict, r.StartedAt})
	}

	if receiptJSONOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}
	if len(rows) == 0 {
		fmt.Println("no receipts in archive")
		return nil
	}
	for _, r := range rows {
		started := r.StartedAt
		if len(started) > 19 {
			started = started[:19]
		}
		fmt.Printf("%s  %-24s %-12s %s\n", started, r.ReceiptID, r.Verdict, r.TaskID)
	}
	return nil
}

func runReceiptShow(cmd *cobra.Command, args []string) error {
	baseDir := receiptDir
	if baseDir == "" {
		var err error
		baseDir, err = findBaseDir(globalCfgFile)
		if err != nil {
			return fmt.Errorf("cannot find base dir: %w", err)
		}
	}

	path := resolveReceiptPath(args[0], baseDir)
	res := verifyReceiptFile(path, baseDir, receiptWantAnchor, receiptPubKey)

	data, err := os.ReadFile(path)
	var r map[string]any
	if err == nil {
		_ = json.Unmarshal(data, &r)
	}

	switch receiptFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	case "markdown":
		fmt.Print(renderReceiptMarkdown(r, res, false))
	case "github":
		fmt.Print(renderReceiptMarkdown(r, res, true))
	case "text":
		printReceiptText(r, res)
	default:
		return fmt.Errorf("unknown format %q (use text, markdown, github, or json)", receiptFormat)
	}
	return nil
}

func receiptField(r map[string]any, key string) string {
	if r == nil {
		return ""
	}
	if v, ok := r[key].(string); ok {
		return v
	}
	return ""
}

func receiptFieldBool(r map[string]any, key string) bool {
	if v, ok := r[key].(bool); ok {
		return v
	}
	return false
}

func receiptFieldInt(r map[string]any, key string) int64 {
	if v, ok := r[key].(float64); ok {
		return int64(v)
	}
	return 0
}

func receiptFieldList(r map[string]any, key string) []string {
	raw, ok := r[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// integritySummary renders the hash/signature/anchor states compactly. It is
// deliberately blunt: anything not proven OK is shown as not OK.
func integritySummary(res *verifyResult) string {
	sig := res.SignatureState
	if res.SignatureOK {
		sig = "OK"
	}
	hash := res.HashState
	if res.HashOK {
		hash = "OK"
	}
	s := fmt.Sprintf("hash: %s, signature: %s", hash, sig)
	if res.AnchorOK == nil {
		s += ", anchor: skipped"
	} else if *res.AnchorOK {
		s += ", anchor: OK"
	} else {
		s += ", anchor: FAILED"
	}
	return s
}

func printReceiptText(r map[string]any, res *verifyResult) {
	fmt.Printf("Receipt:    %s\n", res.ReceiptID)
	if task := receiptField(r, "task_id"); task != "" {
		fmt.Printf("Task:       %s (%s)\n", task, receiptField(r, "goal"))
	}
	fmt.Printf("Verdict:    %s\n", res.Verdict)
	if receiptFieldBool(r, "verdict_overridden") {
		fmt.Printf("Override:   %s -> %s (%s)\n",
			receiptField(r, "initial_verdict"), receiptField(r, "final_verdict"), receiptField(r, "override_reason"))
	}
	fmt.Printf("Integrity:  %s\n", integritySummary(res))
	if hits := receiptFieldList(r, "safety_hits"); len(hits) > 0 {
		fmt.Printf("Safety hits: %d\n", len(hits))
		for _, h := range hits {
			fmt.Printf("  - %s\n", h)
		}
	} else {
		fmt.Println("Safety hits: none")
	}
	if claims := receiptFieldList(r, "pinocchio_false_claims"); len(claims) > 0 {
		fmt.Println("False claims:")
		for _, c := range claims {
			fmt.Printf("  - %s\n", c)
		}
	}
	fmt.Printf("Changes:    %d files, %d patch lines, tests passed: %d\n",
		receiptFieldInt(r, "files_changed"), receiptFieldInt(r, "patch_lines"), receiptFieldInt(r, "tests_passed"))
	fmt.Printf("Result:     %s\n", map[bool]string{true: "VALID", false: "INVALID"}[res.Valid])
	for _, e := range res.Errors {
		fmt.Printf("  - %s\n", e)
	}
	fmt.Printf("Reproduce:  selo verify %s\n", res.ReceiptPathHint())
}

// renderReceiptMarkdown produces a decision card suitable for a PR comment or
// a GitHub Actions job summary. When github is true, verbose sections are
// wrapped in <details> so the card stays scannable.
func renderReceiptMarkdown(r map[string]any, res *verifyResult, github bool) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Selo receipt `%s`\n\n", res.ReceiptID))

	sig := ":x:"
	if res.SignatureOK {
		sig = ":white_check_mark:"
	}
	hash := ":x:"
	if res.HashOK {
		hash = ":white_check_mark:"
	}
	b.WriteString(fmt.Sprintf("**Verdict: `%s`** — integrity: hash %s, signature %s", res.Verdict, hash, sig))
	if res.AnchorOK != nil {
		if *res.AnchorOK {
			b.WriteString(", anchor :white_check_mark:")
		} else {
			b.WriteString(", anchor :x:")
		}
	}
	b.WriteString("\n\n")

	if goal := receiptField(r, "goal"); goal != "" {
		b.WriteString(fmt.Sprintf("Task: `%s` — %s\n\n", receiptField(r, "task_id"), goal))
	}
	b.WriteString(fmt.Sprintf("Files changed: %d · Patch lines: %d · Tests passed: %d · Runner: %s\n\n",
		receiptFieldInt(r, "files_changed"), receiptFieldInt(r, "patch_lines"),
		receiptFieldInt(r, "tests_passed"), receiptField(r, "runner_mode")))

	if hits := receiptFieldList(r, "safety_hits"); len(hits) > 0 {
		b.WriteString(fmt.Sprintf("**Safety findings (%d):**\n\n", len(hits)))
		for _, h := range hits {
			b.WriteString(fmt.Sprintf("- %s\n", h))
		}
		b.WriteString("\n")
	} else {
		b.WriteString("Safety findings: none\n\n")
	}
	if claims := receiptFieldList(r, "pinocchio_false_claims"); len(claims) > 0 {
		b.WriteString("**False claims detected:**\n\n")
		for _, c := range claims {
			b.WriteString(fmt.Sprintf("- %s\n", c))
		}
		b.WriteString("\n")
	}
	if receiptFieldBool(r, "verdict_overridden") {
		b.WriteString(fmt.Sprintf("> Verdict overridden: `%s` -> `%s` (%s)\n\n",
			receiptField(r, "initial_verdict"), receiptField(r, "final_verdict"), receiptField(r, "override_reason")))
	}

	if !res.Valid {
		b.WriteString("**Integrity result: INVALID — do not trust this receipt.**\n\n")
		for _, e := range res.Errors {
			b.WriteString(fmt.Sprintf("- %s\n", e))
		}
		b.WriteString("\n")
	}

	details := func(title, body string) {
		if body == "" {
			return
		}
		if github {
			b.WriteString(fmt.Sprintf("<details>\n<summary>%s</summary>\n\n%s\n</details>\n\n", title, body))
		} else {
			b.WriteString(fmt.Sprintf("**%s**\n\n%s\n", title, body))
		}
	}
	var meta strings.Builder
	if base := receiptField(r, "base_commit"); base != "" {
		meta.WriteString(fmt.Sprintf("- Base commit: `%s`\n", base))
	}
	if wt := receiptField(r, "worktree_path"); wt != "" {
		meta.WriteString(fmt.Sprintf("- Worktree: `%s`\n", wt))
	}
	if started := receiptField(r, "started_at"); started != "" {
		meta.WriteString(fmt.Sprintf("- Started: %s, duration: %ds\n", started, receiptFieldInt(r, "duration_sec")))
	}
	details("Run metadata", strings.TrimRight(meta.String(), "\n"))
	if diff := receiptField(r, "diff_summary"); diff != "" {
		details("Diff (summary)", "```diff\n"+diff+"\n```")
	}

	b.WriteString(fmt.Sprintf("Verify independently: `selo verify %s`\n", res.ReceiptPathHint()))
	return b.String()
}
