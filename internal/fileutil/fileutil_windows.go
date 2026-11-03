package fileutil

import (
	"errors"
	"fmt"
	"os"
)

func replaceAtomic(source, target string) error {
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove old target: %w", err)
	}
	if err := os.Rename(source, target); err != nil {
		return fmt.Errorf("replace target: %w", err)
	}
	return nil
}

func replaceExecutable(target string, data []byte) error {
	old := target + ".old"
	if err := os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove old executable backup: %w", err)
	}
	if err := os.Rename(target, old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("rename old executable: %w", err)
	}
	return WriteAtomic(target, data, 0o755)
}
