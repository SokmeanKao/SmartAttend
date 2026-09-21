package employee

import "testing"

func TestNormalizeEmployeeCode(t *testing.T) {
	if got := NormalizeEmployeeCode(" emp001 "); got != "EMP001" {
		t.Fatalf("NormalizeEmployeeCode() = %q, want %q", got, "EMP001")
	}
}
