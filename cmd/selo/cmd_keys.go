package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/desmondkam/selo/internal/receipt"
	"github.com/spf13/cobra"
)

var (
	keysOut   string
	keysForce bool
	keysPubIn string
)

var keysCmd = &cobra.Command{
	Use:   "keys",
	Short: "Manage the receipt signing key",
	Long: `Selo signs every receipt with Ed25519. Without SELO_SIGNING_KEY an
ephemeral per-process key is used, and signatures cannot be attributed across
runs. Generate a key once and export it to make every receipt verifiable.`,
}

var keysGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate an Ed25519 signing key for receipts",
	RunE:  runKeysGenerate,
}

var keysPubCmd = &cobra.Command{
	Use:   "pub",
	Short: "Print the public key for a signing key file",
	RunE:  runKeysPub,
}

func init() {
	keysGenerateCmd.Flags().StringVar(&keysOut, "out", defaultSigningKeyPath(), "Where to write the base64 seed")
	keysGenerateCmd.Flags().BoolVar(&keysForce, "force", false, "Overwrite an existing key file")
	keysPubCmd.Flags().StringVar(&keysPubIn, "in", defaultSigningKeyPath(), "Key file to read the seed from")
	keysCmd.AddCommand(keysGenerateCmd, keysPubCmd)
}

func defaultSigningKeyPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".selo/signing-key"
	}
	return filepath.Join(home, ".selo", "signing-key")
}

func runKeysGenerate(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(keysOut); err == nil && !keysForce {
		return fmt.Errorf("%s already exists (use --force to overwrite — a new key changes what future receipts are signed with)", keysOut)
	}
	seedB64, pubB64, _, err := receipt.GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("generate keypair: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keysOut), 0700); err != nil {
		return fmt.Errorf("create key directory: %w", err)
	}
	if err := os.WriteFile(keysOut, []byte(seedB64+"\n"), 0600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}
	fmt.Printf("Signing key written to %s (mode 0600)\n\n", keysOut)
	fmt.Printf("Make every receipt attributable across runs:\n\n  export SELO_SIGNING_KEY=$(cat %s)\n\n", keysOut)
	fmt.Printf("Public key (share this; verifiers never need the seed):\n  %s\n\n", pubB64)
	fmt.Printf("Receipts carry the matching public key, so anyone holding a receipt\n")
	fmt.Printf("can check it against this key with: selo verify <receipt.json>\n")
	return nil
}

func runKeysPub(cmd *cobra.Command, args []string) error {
	data, err := os.ReadFile(keysPubIn)
	if err != nil {
		return fmt.Errorf("read key: %w", err)
	}
	os.Setenv("SELO_SIGNING_KEY", strings.TrimSpace(string(data)))
	_, pubB64, err := receipt.LoadSigningKey()
	if err != nil {
		return fmt.Errorf("load key: %w", err)
	}
	fmt.Println(pubB64)
	return nil
}
