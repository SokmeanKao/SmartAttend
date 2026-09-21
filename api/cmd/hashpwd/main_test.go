package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/smartattend/api/internal/auth"
)

func TestRunReadsPasswordFromStdin(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"hashpwd"}, strings.NewReader("secret password\n"), &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr = %s", code, stderr.String())
	}
	hash := strings.TrimSpace(stdout.String())
	if !auth.VerifyPassword(hash, "secret password") {
		t.Error("hash does not verify password read from stdin")
	}
}

func TestRunRejectsPasswordArgument(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"hashpwd", "leaked-secret"}, strings.NewReader(""), &stdout, &stderr)

	if code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if strings.Contains(stderr.String(), "leaked-secret") {
		t.Error("stderr exposes password argument")
	}
}
