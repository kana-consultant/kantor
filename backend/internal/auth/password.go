package auth

import "golang.org/x/crypto/bcrypt"

// PasswordHashCost is the bcrypt cost factor used when hashing new passwords.
// OWASP currently recommends a cost of 12 or higher for bcrypt; the library
// default is only 10. Existing hashes at lower costs continue to verify, and
// callers may use NeedsRehash to opportunistically upgrade them on next login.
const PasswordHashCost = 12

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), PasswordHashCost)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func ComparePassword(hash string, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// NeedsRehash reports whether an existing bcrypt hash was produced with a cost
// lower than the current PasswordHashCost. Callers that have just verified a
// password successfully can use this signal to re-hash and persist the new
// digest, transparently upgrading legacy hashes over time.
func NeedsRehash(hash string) bool {
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		return false
	}
	return cost < PasswordHashCost
}
