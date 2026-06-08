package secrets

import (
	"os"
	"strings"
)

// Get reads a secret: checks the environment variable first,
// then falls back to reading the file at /run/secrets/<name>.
func Get(envKey, fileName string) string {
	if v := os.Getenv(envKey); v != "" {
		return strings.TrimSpace(v)
	}

	data, err := os.ReadFile("/run/secrets/" + fileName)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}
