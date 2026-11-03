package hooksjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"

	"github.com/chilogen/jit-rules/internal/harness"
	"github.com/chilogen/jit-rules/internal/installer"
	"github.com/chilogen/jit-rules/internal/paths"
)

const kind = "hooks-json"

type implementation struct{}

func init() {
	installer.Register(implementation{})
}

func (implementation) Kind() string { return kind }

func (implementation) Plan(req installer.Request) ([]installer.Change, error) {
	targets, err := installer.TargetPaths(req.Def, req.Scope, req.Root)
	if err != nil {
		return nil, err
	}
	changes := make([]installer.Change, 0, len(targets))
	for _, target := range targets {
		change, err := planTarget(req, target)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(change.Before, change.After) {
			changes = append(changes, change)
		}
	}
	return changes, nil
}

func (implementation) Installed(req installer.Request) (bool, error) {
	targets, err := installedTargets(req)
	if err != nil {
		return false, err
	}
	for _, target := range targets {
		data, err := os.ReadFile(target)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, fmt.Errorf("read %s: %w", target, err)
		}
		root, err := decodeObject(data)
		if err != nil {
			return false, fmt.Errorf("parse %s: %w", target, err)
		}
		if hasInstalledHandler(root) {
			return true, nil
		}
	}
	return false, nil
}

func planTarget(req installer.Request, target string) (installer.Change, error) {
	before, err := os.ReadFile(target)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return installer.Change{}, fmt.Errorf("read %s: %w", target, err)
	}
	root, err := initialObject(before)
	if err != nil {
		return installer.Change{}, fmt.Errorf("parse %s: %w", target, err)
	}
	removeHandlers(root)
	if req.Uninstall {
		after, err := uninstallResult(root, req.Def.Install.FileDefaults)
		return installer.Change{Path: target, Before: before, After: after}, err
	}
	addHandlers(root, req)
	addDefaults(root, req.Def.Install.FileDefaults)
	after, err := encodeObject(root)
	return installer.Change{Path: target, Before: before, After: after}, err
}

func initialObject(data []byte) (*object, error) {
	if data == nil {
		return &object{}, nil
	}
	return decodeObject(data)
}

func addHandlers(root *object, req installer.Request) {
	hooks, ok := root.get("hooks")
	hookObject, valid := hooks.(*object)
	if !ok || !valid {
		hookObject = &object{}
		root.set("hooks", hookObject)
	}
	goos := req.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	command := harness.CommandString(req.Def, req.Bin, goos)
	entries := harness.HookEntries(req.Def, command)
	events := make([]string, 0, len(entries))
	for event := range entries {
		events = append(events, event)
	}
	sort.Strings(events)
	for _, event := range events {
		current, _ := hookObject.get(event)
		groups, _ := current.([]any)
		for _, entry := range entries[event] {
			groups = append(groups, normalize(entry))
		}
		hookObject.set(event, groups)
	}
}

func addDefaults(root *object, defaults map[string]any) {
	normalized := normalize(defaults).(*object)
	for _, item := range normalized.members {
		if _, exists := root.get(item.key); !exists {
			root.set(item.key, item.value)
		}
	}
}

func uninstallResult(root *object, defaults map[string]any) ([]byte, error) {
	if onlyDefaults(root, defaults) {
		return nil, nil
	}
	return encodeObject(root)
}

func onlyDefaults(root *object, defaults map[string]any) bool {
	if len(root.members) == 0 {
		return true
	}
	for _, item := range root.members {
		expected, ok := defaults[item.key]
		if !ok || !jsonEqual(item.value, normalize(expected)) {
			return false
		}
	}
	return true
}

func jsonEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil &&
		reflect.DeepEqual(leftJSON, rightJSON)
}

func installedTargets(req installer.Request) ([]string, error) {
	target := req.Def.Install.Targets[req.Scope]
	if target.Mode != "existing" {
		return installer.TargetPaths(req.Def, req.Scope, req.Root)
	}
	base := ""
	if req.Scope == "project" {
		base = req.Root
	}
	var result []string
	for _, value := range target.Paths {
		targetPath := paths.Expand(value, base)
		if info, err := os.Stat(filepath.Dir(targetPath)); err == nil && info.IsDir() {
			result = append(result, targetPath)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect config directory: %w", err)
		}
	}
	return result, nil
}

func isJITRHandler(value any) bool {
	handler, ok := value.(*object)
	if !ok {
		return false
	}
	command, ok := handler.get("command")
	text, ok := command.(string)
	return ok && strings.Contains(text, "jitr") &&
		strings.Contains(text, " hook --harness ")
}
