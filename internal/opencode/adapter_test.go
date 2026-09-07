package opencode

import (
	"testing"
)

func TestValidateFilePath_EmptyAllowlist(t *testing.T) {
	err := ValidateFilePath("/any/path.go", nil)
	if err != nil {
		t.Errorf("empty allowlist should permit all paths, got: %v", err)
	}
}

func TestValidateFilePath_ExactMatch(t *testing.T) {
	allowlist := []string{"/project/src/main.go"}
	err := ValidateFilePath("/project/src/main.go", allowlist)
	if err != nil {
		t.Errorf("exact match should pass, got: %v", err)
	}
}

func TestValidateFilePath_NoMatch(t *testing.T) {
	allowlist := []string{"/project/src/*.go"}
	err := ValidateFilePath("/project/secret/key.pem", allowlist)
	if err == nil {
		t.Error("non-matching path should fail")
	}
}

func TestValidateFilePath_GlobMatch(t *testing.T) {
	allowlist := []string{"/project/src/*.go"}
	err := ValidateFilePath("/project/src/main.go", allowlist)
	if err != nil {
		t.Errorf("glob match should pass, got: %v", err)
	}
}

func TestValidateFilePath_DirectoryPrefix(t *testing.T) {
	allowlist := []string{"/project/src/"}
	err := ValidateFilePath("/project/src/main.go", allowlist)
	if err != nil {
		t.Errorf("directory prefix match should pass, got: %v", err)
	}
}

func TestValidateFilePath_WildcardDirectory(t *testing.T) {
	allowlist := []string{"/project/*"}
	err := ValidateFilePath("/project/src/main.go", allowlist)
	if err != nil {
		t.Errorf("wildcard directory match should pass, got: %v", err)
	}
}
