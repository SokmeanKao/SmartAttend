package auth

import "testing"

const knownPasswordHash = "$argon2id$v=19$m=65536,t=1,p=12$3XMoKQWRevFuU7xwlYJkeA$7HS15wjxGdiO97QGiVdvlWq0bPXdqonWOHn3GY7O7bc"

func TestVerifyPassword(t *testing.T) {
	if !VerifyPassword(knownPasswordHash, "correct horse battery staple") {
		t.Error("VerifyPassword() = false for matching password")
	}
	if VerifyPassword(knownPasswordHash, "wrong") {
		t.Error("VerifyPassword() = true for wrong password")
	}
	if VerifyPassword("not-an-argon2id-hash", "anything") {
		t.Error("VerifyPassword() = true for malformed hash")
	}
}
