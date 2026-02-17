package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const GitCryptAttributesPattern = "secrets.*.yaml filter=git-crypt diff=git-crypt"

// EnsureGitCryptAttributes ensures the .gitattributes file in outputDir contains
// the git-crypt pattern for secrets files. It creates the file if missing and
// appends the pattern if not already present.
func EnsureGitCryptAttributes(outputDir string) error {
	path := filepath.Join(outputDir, ".gitattributes")

	data, err := os.ReadFile(path) // #nosec G304
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}

	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == GitCryptAttributesPattern {
			return nil // already present
		}
	}

	// Append the pattern
	suffix := "\n"
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		suffix = "\n\n"
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600) // #nosec G304
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", path, err)
	}

	if len(content) == 0 {
		suffix = ""
	}

	if _, err := fmt.Fprintf(f, "%s%s\n", suffix, GitCryptAttributesPattern); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to write %s: %w", path, err)
	}

	return f.Close()
}
