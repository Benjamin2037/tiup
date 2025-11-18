package command

import (
	"os"
	"testing"
)

func TestBuildPrecheckTiDBCredentialsDefaults(t *testing.T) {
	creds, err := buildPrecheckTiDBCredentials("", "", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.user != "root" {
		t.Fatalf("expected default user 'root', got %q", creds.user)
	}
	if creds.password != "" {
		t.Fatalf("expected empty password, got %q", creds.password)
	}
}

func TestBuildPrecheckTiDBCredentialsFromFile(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "pwd")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	if _, err := tmp.WriteString("secret\n"); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	tmp.Close()

	creds, err := buildPrecheckTiDBCredentials(" tidb ", "", tmp.Name(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.user != "tidb" {
		t.Fatalf("expected trimmed user 'tidb', got %q", creds.user)
	}
	if creds.password != "secret" {
		t.Fatalf("expected password 'secret', got %q", creds.password)
	}
}

func TestBuildPrecheckTiDBCredentialsConflicts(t *testing.T) {
	if _, err := buildPrecheckTiDBCredentials("root", "a", "b", false); err == nil {
		t.Fatal("expected error when both password and password-file are set")
	}
	tmp, err := os.CreateTemp(t.TempDir(), "pwd")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	tmp.Close()
	if _, err := buildPrecheckTiDBCredentials("root", "", tmp.Name(), true); err == nil {
		t.Fatal("expected error when prompt and password-file are both set")
	}
}
