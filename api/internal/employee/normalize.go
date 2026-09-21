package employee

import "strings"

func NormalizeEmployeeCode(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}
