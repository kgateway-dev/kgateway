package fsutils

import (
	"fmt"
	"os"
)

// ToTempFile takes a string to write to a temp file. It returns the filename and an error.
func ToTempFile(content string) (string, error) {
	f, err := os.CreateTemp("", "")
	if err != nil {
		return "", err
	}
	defer f.Close()

	n, err := f.WriteString(content)
	if err != nil {
		return "", err
	}

	if n != len(content) {
		return "", fmt.Errorf("expected to write %d bytes, actually wrote %d", len(content), n)
	}
	return f.Name(), nil
}

// IsDirectory checks the provided path is a directory by first checking something exists at that path
// and then checking that it is a directory.
func IsDirectory(dir string) bool {
	stat, err := os.Stat(dir)
	if err != nil {
		return false
	}
	return stat.IsDir()
}
