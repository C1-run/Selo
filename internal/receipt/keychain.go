package receipt

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// DefaultKeychainService is the OS-keychain entry Selo stores the signing seed
// under when SELO_SIGNER=keychain.
const DefaultKeychainService = "selo-signing-key"

const keychainAccount = "selo"

// KeychainGet reads the signing seed from the OS keychain.
//
// macOS: the login keychain via `security`. Linux: the Secret Service via
// `secret-tool`. No CGO, so the static release binaries keep working.
func KeychainGet(service string) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("security", "find-generic-password", "-s", service, "-a", keychainAccount, "-w").Output()
		if err != nil {
			return "", fmt.Errorf("no keychain entry for service %q", service)
		}
		return strings.TrimSpace(string(out)), nil
	case "linux":
		if _, err := exec.LookPath("secret-tool"); err != nil {
			return "", fmt.Errorf("secret-tool not found (install libsecret-tools to use the keychain backend)")
		}
		out, err := exec.Command("secret-tool", "lookup", "service", service, "account", keychainAccount).Output()
		if err != nil {
			return "", fmt.Errorf("no keychain entry for service %q", service)
		}
		return strings.TrimSpace(string(out)), nil
	default:
		return "", fmt.Errorf("no keychain backend on %s", runtime.GOOS)
	}
}

// KeychainSet stores the signing seed in the OS keychain, replacing any
// existing entry.
func KeychainSet(service, secret string) error {
	switch runtime.GOOS {
	case "darwin":
		// -U updates an existing item instead of failing.
		cmd := exec.Command("security", "add-generic-password", "-s", service, "-a", keychainAccount, "-w", secret, "-U")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("keychain store failed: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	case "linux":
		if _, err := exec.LookPath("secret-tool"); err != nil {
			return fmt.Errorf("secret-tool not found (install libsecret-tools to use the keychain backend)")
		}
		cmd := exec.Command("secret-tool", "store", "--label", "Selo signing key", "service", service, "account", keychainAccount)
		cmd.Stdin = strings.NewReader(secret)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("keychain store failed: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	default:
		return fmt.Errorf("no keychain backend on %s", runtime.GOOS)
	}
}

// KeychainDelete removes the signing seed from the OS keychain.
func KeychainDelete(service string) error {
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("security", "delete-generic-password", "-s", service, "-a", keychainAccount)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("keychain delete failed: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	case "linux":
		if _, err := exec.LookPath("secret-tool"); err != nil {
			return fmt.Errorf("secret-tool not found")
		}
		cmd := exec.Command("secret-tool", "clear", "service", service, "account", keychainAccount)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("keychain delete failed: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	default:
		return fmt.Errorf("no keychain backend on %s", runtime.GOOS)
	}
}
