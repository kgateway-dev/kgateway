package fsutils

import (
	"fmt"
	"os"
)

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

func IsDirectory(dir string) bool {
	stat, err := os.Stat(dir)
	if err != nil {
		return false
	}
	return stat.IsDir()
}
