// Package installer plans and applies harness configuration changes.
package installer

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chilogen/jit-rules/internal/fileutil"
	"github.com/chilogen/jit-rules/internal/harness"
	"github.com/chilogen/jit-rules/internal/paths"
)

// Change is one planned file replacement or deletion.
type Change struct {
	Path   string
	Before []byte
	After  []byte
}

// Request describes an install or uninstall operation.
type Request struct {
	Def       *harness.Definition
	Scope     string
	Root      string
	Bin       string
	Uninstall bool
	GOOS      string
}

// Installer plans one data-defined installation kind.
type Installer interface {
	Kind() string
	Plan(req Request) ([]Change, error)
	Installed(req Request) (bool, error)
}

var installers = struct {
	sync.RWMutex
	values map[string]Installer
}{values: make(map[string]Installer)}

// Register makes an installer available by kind.
func Register(i Installer) {
	installers.Lock()
	defer installers.Unlock()
	installers.values[i.Kind()] = i
}

// Get looks up a registered installer.
func Get(kind string) (Installer, bool) {
	installers.RLock()
	defer installers.RUnlock()
	value, ok := installers.values[kind]
	return value, ok
}

// TargetPaths resolves the files selected by a harness target.
func TargetPaths(def *harness.Definition, scope, root string) ([]string, error) {
	target, ok := def.Install.Targets[scope]
	if !ok {
		return nil, fmt.Errorf("%s 不支持项目级安装", def.Title)
	}
	base := ""
	if scope == "project" {
		base = root
	}
	resolved := make([]string, 0, len(target.Paths))
	for _, path := range target.Paths {
		resolved = append(resolved, paths.Expand(path, base))
	}
	switch target.Mode {
	case "first":
		if len(resolved) == 0 {
			return nil, fmt.Errorf("%s 没有安装目标", def.Title)
		}
		return resolved[:1], nil
	case "existing":
		existing, err := pathsWithExistingParents(resolved)
		if err != nil {
			return nil, err
		}
		if len(existing) == 0 {
			return nil, fmt.Errorf("没有找到 %s 的配置目录，请先安装并启动一次 %s", def.Title, def.Title)
		}
		return existing, nil
	default:
		return nil, fmt.Errorf("%s 的安装目标模式 %q 无效", def.Title, target.Mode)
	}
}

// Apply backs up and applies planned changes.
func Apply(changes []Change, now time.Time) error {
	for _, change := range changes {
		if bytes.Equal(change.Before, change.After) {
			continue
		}
		if change.Before != nil {
			if err := fileutil.Backup(change.Path, now); err != nil {
				return fmt.Errorf("backup %s: %w", change.Path, err)
			}
		}
		if change.After == nil {
			if err := os.Remove(change.Path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove %s: %w", change.Path, err)
			}
			continue
		}
		if err := fileutil.WriteAtomic(change.Path, change.After, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", change.Path, err)
		}
	}
	return nil
}

func pathsWithExistingParents(candidates []string) ([]string, error) {
	var result []string
	for _, candidate := range candidates {
		info, err := os.Stat(filepath.Dir(candidate))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect config directory: %w", err)
		}
		if info.IsDir() {
			result = append(result, candidate)
		}
	}
	return result, nil
}
