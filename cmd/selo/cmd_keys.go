package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/C1-run/selo/internal/receipt"
	"github.com/spf13/cobra"
)

var (
	keysOut        string
	keysForce      bool
	keysPubIn      string
	keysKeychain   bool
	keysStoreIn    string
	keysDeleteFile bool
)

var keysCmd = &cobra.Command{
	Use:   "keys",
	Short: "Manage the receipt signing key",
	Long: `Selo signs every receipt with Ed25519. "selo keys generate" writes a key to
~/.selo/signing-key, which Selo then loads automatically — no env var needed.
Without any key, Selo refuses to sign (fail-closed) unless you opt into an
ephemeral per-process key with --dev or SELO_ALLOW_EPHEMERAL_KEY=1.

To keep the key off the filesystem the audited agent can read, store it in the
OS keychain instead: "selo keys store --keychain", then export SELO_SIGNER=keychain.
To put the signer entirely outside the agent's trust domain, use an external
signer: SELO_SIGNER=command (see ADR-008).`,
}

var keysGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate an Ed25519 signing key for receipts",
	RunE:  runKeysGenerate,
}

var keysPubCmd = &cobra.Command{
	Use:   "pub",
	Short: "Print the public key for a signing key",
	RunE:  runKeysPub,
}

var keysStoreCmd = &cobra.Command{
	Use:   "store",
	Short: "Move the signing key into the OS keychain",
	Long: `store reads the signing seed from a file and writes it to the OS keychain
(macOS Keychain, or the Secret Service via secret-tool on Linux). With
SELO_SIGNER=keychain, Selo then signs from the keychain, so no plaintext seed
sits on the filesystem the agent can read.

This raises the bar but is not full separation: the keychain is still readable
by the same user. See ADR-008 for the trust-domain options.`,
	RunE: runKeysStore,
}

func init() {
	keysGenerateCmd.Flags().StringVar(&keysOut, "out", defaultSigningKeyPath(), "Where to write the base64 seed")
	keysGenerateCmd.Flags().BoolVar(&keysForce, "force", false, "Overwrite an existing key file")
	keysGenerateCmd.Flags().BoolVar(&keysKeychain, "keychain", false, "Store the key in the OS keychain instead of a file")

	keysPubCmd.Flags().StringVar(&keysPubIn, "in", defaultSigningKeyPath(), "Key file to read the seed from")
	keysPubCmd.Flags().BoolVar(&keysKeychain, "keychain", false, "Read the seed from the OS keychain")

	keysStoreCmd.Flags().StringVar(&keysStoreIn, "in", "", "Key file to read the seed from (default: ~/.selo/signing-key)")
	keysStoreCmd.Flags().BoolVar(&keysDeleteFile, "delete-file", false, "Delete the plaintext key file after storing it in the keychain")

	keysCmd.AddCommand(keysGenerateCmd, keysPubCmd, keysStoreCmd)
}

func defaultSigningKeyPath() string {
	return receipt.DefaultSigningKeyPath()
}

func runKeysGenerate(cmd *cobra.Command, args []string) error {
	seedB64, pubB64, _, err := receipt.GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("generate keypair: %w", err)
	}

	if keysKeychain {
		if err := receipt.KeychainSet(receipt.DefaultKeychainService, seedB64); err != nil {
			return err
		}
		fmt.Printf("Signing key stored in the OS keychain (service %q).\n\n", receipt.DefaultKeychainService)
		fmt.Printf("Use it with:\n\n  export SELO_SIGNER=keychain\n\n")
		fmt.Printf("Public key (share this; verifiers never need the seed):\n  %s\n\n", pubB64)
		return nil
	}

	if _, err := os.Stat(keysOut); err == nil && !keysForce {
		return fmt.Errorf("%s already exists (use --force to overwrite — a new key changes what future receipts are signed with)", keysOut)
	}
	if err := os.MkdirAll(filepath.Dir(keysOut), 0700); err != nil {
		return fmt.Errorf("create key directory: %w", err)
	}
	if err := os.WriteFile(keysOut, []byte(seedB64+"\n"), 0600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}
	fmt.Printf("Signing key written to %s (mode 0600)\n\n", keysOut)
	if keysOut == receipt.DefaultSigningKeyPath() {
		fmt.Printf("Selo loads this key automatically — no env var needed.\n")
		fmt.Printf("NOTE: this file is readable by any process running as this user,\n")
		fmt.Printf("including the agent. For a key the agent cannot read, run\n")
		fmt.Printf("`selo keys store --keychain` and use SELO_SIGNER=keychain.\n\n")
	} else {
		fmt.Printf("Make every receipt attributable across runs:\n\n  export SELO_SIGNING_KEY=$(cat %s)\n\n", keysOut)
	}
	fmt.Printf("Public key (share this; verifiers never need the seed):\n  %s\n\n", pubB64)
	fmt.Printf("Receipts carry the matching public key, so anyone holding a receipt\n")
	fmt.Printf("can check it against this key with: selo verify <receipt.json>\n")
	return nil
}

func runKeysPub(cmd *cobra.Command, args []string) error {
	var seed string
	if keysKeychain {
		s, err := receipt.KeychainGet(receipt.DefaultKeychainService)
		if err != nil {
			return err
		}
		seed = s
	} else {
		data, err := os.ReadFile(keysPubIn)
		if err != nil {
			return fmt.Errorf("read key: %w", err)
		}
		seed = strings.TrimSpace(string(data))
	}
	os.Setenv("SELO_SIGNING_KEY", seed)
	_, pubB64, err := receipt.LoadSigningKey()
	if err != nil {
		return fmt.Errorf("load key: %w", err)
	}
	fmt.Println(pubB64)
	return nil
}

func runKeysStore(cmd *cobra.Command, args []string) error {
	path := keysStoreIn
	if path == "" {
		path = receipt.DefaultSigningKeyPath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read key file %s: %w", path, err)
	}
	secret := strings.TrimSpace(string(data))
	if secret == "" {
		return fmt.Errorf("%s is empty", path)
	}
	if err := receipt.KeychainSet(receipt.DefaultKeychainService, secret); err != nil {
		return err
	}
	fmt.Printf("Stored the signing key in the OS keychain (service %q).\n", receipt.DefaultKeychainService)
	if keysDeleteFile {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("stored in the keychain but could not delete %s: %w", path, err)
		}
		fmt.Printf("Removed the plaintext key file %s.\n", path)
	} else {
		fmt.Printf("NOTE: the plaintext key file %s is still on disk; remove it (or re-run with --delete-file).\n", path)
	}
	fmt.Printf("\nUse it with:\n\n  export SELO_SIGNER=keychain\n")
	return nil
}
