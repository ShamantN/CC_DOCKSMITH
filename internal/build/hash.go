package build

import (
	"crypto/sha256"
	"fmt"
)

// sha256hex returns the lowercase hex SHA-256 digest of the given bytes.
// Used internally when a synthetic digest is needed (e.g. for bare base images).
func sha256hex(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}
