package registry

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chilogen/jit-rules/internal/fileutil"
	"github.com/chilogen/jit-rules/internal/paths"
	"gopkg.in/yaml.v3"
)

type registryFile struct {
	Version  int     `yaml:"version"`
	Profiles []Entry `yaml:"profiles"`
}

func load() ([]Entry, error) {
	data, err := os.ReadFile(filepath.Join(paths.Config(), "registry.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}
	var file registryFile
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}
	if file.Version != 1 {
		return nil, fmt.Errorf("unsupported registry version %d", file.Version)
	}
	if file.Profiles == nil {
		file.Profiles = []Entry{}
	}
	if err := validate(file.Profiles); err != nil {
		return nil, err
	}
	return file.Profiles, nil
}

func save(entries []Entry) error {
	if err := validate(entries); err != nil {
		return err
	}
	data, err := yaml.Marshal(registryFile{Version: 1, Profiles: entries})
	if err != nil {
		return err
	}
	return fileutil.WriteAtomic(filepath.Join(paths.Config(), "registry.yaml"), data, 0o600)
}

type rankedEntry struct {
	entry Entry
	index int
}

func applicable(entries []Entry, cwd string) []Entry {
	cwd = filepath.Clean(cwd)
	var scoped, global []rankedEntry
	for index, entry := range entries {
		if isGlobal(entry) {
			global = append(global, rankedEntry{entry: entry, index: index})
			continue
		}
		if pathContains(scope(entry), cwd) {
			scoped = append(scoped, rankedEntry{entry: entry, index: index})
		}
	}
	sort.SliceStable(scoped, func(i, j int) bool {
		left := len(scope(scoped[i].entry))
		right := len(scope(scoped[j].entry))
		return left > right
	})
	sort.Slice(global, func(i, j int) bool { return global[i].index > global[j].index })
	result := make([]Entry, 0, len(scoped)+len(global))
	for _, ranked := range append(scoped, global...) {
		result = append(result, ranked.entry)
	}
	return result
}

func validate(entries []Entry) error {
	for _, entry := range entries {
		if !filepath.IsAbs(entry.Path) {
			return fmt.Errorf("profile 路径必须是绝对路径：%s", entry.Path)
		}
		if filepath.Base(filepath.Clean(entry.Path)) != ".jitr" {
			return fmt.Errorf("profile 必须位于 .jitr 目录：%s", entry.Path)
		}
	}
	return nil
}

func scope(entry Entry) string {
	return filepath.Dir(filepath.Clean(entry.Path))
}

func isGlobal(entry Entry) bool {
	home := paths.Home()
	return home != "" &&
		filepath.Clean(entry.Path) == filepath.Join(filepath.Clean(home), ".jitr")
}

func pathContains(parent, child string) bool {
	relative, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(child))
	return err == nil && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
