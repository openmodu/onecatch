package httptransport

import "fmt"

// Redaction helpers keep secrets and user content out of logs. Anything that
// logs a token, payment reference, or user-authored text must pass through
// these first.

// redactToken masks a bearer/admin/session token, revealing only a short
// suffix so log lines can still be correlated without exposing the secret.
func redactToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 4 {
		return "****"
	}
	return "****" + token[len(token)-4:]
}

// promptDigest summarizes user-authored text (order requirements, etc.) as a
// length only, so the full content never reaches logs.
func promptDigest(prompt string) string {
	return fmt.Sprintf("<%d chars>", len([]rune(prompt)))
}
