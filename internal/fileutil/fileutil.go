// Package fileutil provides durable file replacement helpers.
package fileutil

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// WriteAtomic writes through a temporary file in the destination directory.
func WriteAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(perm); err != nil {
		temp.Close()
		return fmt.Errorf("set temporary file mode: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	return replaceAtomic(tempPath, path)
}

// Backup copies an existing file to a timestamped sibling.
func Backup(path string, now time.Time) error {
	source, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open backup source: %w", err)
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return fmt.Errorf("stat backup source: %w", err)
	}
	target, err := createBackup(path+".bak-jitr-"+now.Format("20060102150405"), info.Mode())
	if err != nil {
		return fmt.Errorf("create backup: %w", err)
	}
	if _, err := io.Copy(target, source); err != nil {
		target.Close()
		return fmt.Errorf("copy backup: %w", err)
	}
	if err := target.Close(); err != nil {
		return fmt.Errorf("close backup: %w", err)
	}
	return nil
}

func createBackup(base string, mode os.FileMode) (*os.File, error) {
	for index := 0; ; index++ {
		path := base
		if index > 0 {
			path = fmt.Sprintf("%s.%d", base, index)
		}
		target, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return target, err
	}
}

// ReplaceExecutable replaces dst with the contents of src.
func ReplaceExecutable(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read replacement executable: %w", err)
	}
	return replaceExecutable(dst, data)
}
