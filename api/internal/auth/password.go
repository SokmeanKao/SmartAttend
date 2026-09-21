package auth

import "github.com/alexedwards/argon2id"

func VerifyPassword(encodedHash, password string) bool {
	match, err := argon2id.ComparePasswordAndHash(password, encodedHash)
	return err == nil && match
}

func HashPassword(password string) (string, error) {
	return argon2id.CreateHash(password, argon2id.DefaultParams)
}
