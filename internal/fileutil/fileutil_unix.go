//go:build !windows

package fileutil

import (
	"fmt"
	"os"
)

func replaceAtomic(source, target string) error {
	if err := os.Rename(source, target); err != nil {
		return fmt.Errorf("replace target: %w", err)
	}
	return nil
}

func replaceExecutable(target string, data []byte) error {
	return WriteAtomic(target, data, 0o755)
}
