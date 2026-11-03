package profile

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/chilogen/jit-rules/internal/fileutil"
	"github.com/chilogen/jit-rules/internal/paths"
	"gopkg.in/yaml.v3"
)

const lockComment = "# 由 jitr 生成。记录哪些单元和包已经生效。不要手动修改。\n"

var profileNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

func load(dir string) (*Profile, error) {
	data, err := os.ReadFile(filepath.Join(dir, "profile.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}
	var profile Profile
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&profile); err != nil {
		return nil, fmt.Errorf("parse profile: %w", err)
	}
	if !profileNamePattern.MatchString(profile.Name) || profile.Name == "core" {
		return nil, fmt.Errorf("invalid profile name %q", profile.Name)
	}
	if profile.Packs == nil {
		return nil, errors.New("profile packs is required")
	}
	if key := plainAPIKey(profile.Settings); key != "" {
		return nil, errors.New("不要在 profile 里写 API key，请改用 api_key_env 指定环境变量名")
	}
	profile.Dir = filepath.Clean(dir)
	return &profile, nil
}

func plainAPIKey(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if strings.HasSuffix(key, "api_key") && key != "api_key_env" {
				return key
			}
			if found := plainAPIKey(child); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := plainAPIKey(child); found != "" {
				return found
			}
		}
	}
	return ""
}

func emptyLock() *Lock {
	return &Lock{
		Version: 1,
		Units:   map[string]LockUnit{},
		Packs:   map[string]LockPack{},
	}
}

func loadLock(dir string) (*Lock, error) {
	data, err := os.ReadFile(filepath.Join(dir, "jitr.lock"))
	if errors.Is(err, os.ErrNotExist) {
		return emptyLock(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read lock: %w", err)
	}
	var lock Lock
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&lock); err != nil {
		return nil, fmt.Errorf("parse lock: %w", err)
	}
	if lock.Version != 1 {
		return nil, fmt.Errorf("unsupported lock version %d", lock.Version)
	}
	if lock.Units == nil {
		lock.Units = map[string]LockUnit{}
	}
	if lock.Packs == nil {
		lock.Packs = map[string]LockPack{}
	}
	return &lock, nil
}

func saveLock(dir string, lock *Lock) error {
	if lock == nil {
		return errors.New("lock is nil")
	}
	if lock.Units == nil {
		lock.Units = map[string]LockUnit{}
	}
	if lock.Packs == nil {
		lock.Packs = map[string]LockPack{}
	}
	data, err := yaml.Marshal(lock)
	if err != nil {
		return err
	}
	return fileutil.WriteAtomic(
		filepath.Join(dir, "jitr.lock"), append([]byte(lockComment), data...), 0o600,
	)
}

func initProfile(dir, name string) error {
	if !profileNamePattern.MatchString(name) || name == "core" {
		return fmt.Errorf("invalid profile name %q", name)
	}
	profilePath := filepath.Join(dir, "profile.yaml")
	if _, err := os.Stat(profilePath); err == nil {
		return fmt.Errorf("%s already exists", profilePath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "units"), 0o755); err != nil {
		return err
	}
	profileData := []byte(fmt.Sprintf(
		"name: %s\npacks:\n  - name: core\n    source: builtin\nsettings: {}\n", name,
	))
	if err := fileutil.WriteAtomic(profilePath, profileData, 0o600); err != nil {
		return err
	}
	if err := saveLock(dir, emptyLock()); err != nil {
		return err
	}
	root := paths.ProjectRoot(dir)
	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		return nil
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return nil
	}
	command := exec.Command(git, "init")
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("git init: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func commit(dir, message string) string {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return ""
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return "git 提交失败：" + err.Error()
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-m", message}} {
		command := exec.Command(git, args...)
		command.Dir = dir
		if output, err := command.CombinedOutput(); err != nil {
			text := strings.TrimSpace(string(output))
			if text != "" {
				return "git 提交失败：" + text
			}
			return "git 提交失败：" + err.Error()
		}
	}
	return ""
}
