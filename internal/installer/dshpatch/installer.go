package dshpatch

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"github.com/chilogen/jit-rules/internal/installer"
	"github.com/chilogen/jit-rules/internal/paths"
	"gopkg.in/yaml.v3"
)

const (
	kind      = "dsh-patch"
	hooksKind = "hooks-json"
)

type implementation struct{}

func init() {
	installer.Register(implementation{})
}

func (implementation) Kind() string { return kind }

func (implementation) Plan(req installer.Request) ([]installer.Change, error) {
	hookInstaller, ok := installer.Get(hooksKind)
	if !ok {
		return nil, fmt.Errorf("installer %q is not registered", hooksKind)
	}
	hookChanges, err := hookInstaller.Plan(req)
	if err != nil {
		return nil, err
	}
	hookPath, err := hookTargetPath(req, hookChanges)
	if err != nil {
		return nil, err
	}
	patchChange, changed, err := planPatch(req, hookPath)
	if err != nil {
		return nil, err
	}
	if changed {
		hookChanges = append(hookChanges, patchChange)
	}
	return hookChanges, nil
}

func (implementation) Installed(req installer.Request) (bool, error) {
	if _, err := installer.TargetPaths(req.Def, req.Scope, req.Root); err != nil {
		return false, err
	}
	path := paths.Expand(req.Def.Install.Options["patch_file"], "")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	document, err := parsePatch(data)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", path, err)
	}
	_, _, found := findEntry(sequence(document), req.Def.Install.Options["entry_id"])
	return found, nil
}

func hookTargetPath(req installer.Request, changes []installer.Change) (string, error) {
	targets, err := installer.TargetPaths(req.Def, req.Scope, req.Root)
	if err != nil {
		return "", err
	}
	if len(targets) == 0 {
		return "", fmt.Errorf("missing hooks target")
	}
	if len(changes) > 0 {
		return changes[0].Path, nil
	}
	return targets[0], nil
}

func planPatch(req installer.Request, hookPath string) (installer.Change, bool, error) {
	path := paths.Expand(req.Def.Install.Options["patch_file"], "")
	before, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return installer.Change{}, false, fmt.Errorf("read %s: %w", path, err)
	}
	document, err := parsePatch(before)
	if err != nil {
		return installer.Change{}, false, fmt.Errorf("parse %s: %w", path, err)
	}
	root := sequence(document)
	entryID := req.Def.Install.Options["entry_id"]
	if req.Uninstall {
		if _, _, found := findEntry(root, entryID); !found {
			return installer.Change{}, false, nil
		}
		removeEntry(root, entryID)
	} else {
		upsertEntry(root, entryID, req.Def.Install.Options["plugin"], hookPath)
	}
	after, err := yaml.Marshal(document)
	if err != nil {
		return installer.Change{}, false, fmt.Errorf("encode %s: %w", path, err)
	}
	change := installer.Change{Path: path, Before: before, After: after}
	return change, !bytes.Equal(before, after), nil
}

func parsePatch(data []byte) (*yaml.Node, error) {
	if len(data) == 0 {
		return &yaml.Node{
			Kind: yaml.DocumentNode,
			Content: []*yaml.Node{{
				Kind: yaml.SequenceNode,
			}},
		}, nil
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	if sequence(&document) == nil {
		return nil, fmt.Errorf("patch file is not a YAML list")
	}
	return &document, nil
}

func sequence(document *yaml.Node) *yaml.Node {
	if document == nil || document.Kind != yaml.DocumentNode ||
		len(document.Content) != 1 || document.Content[0].Kind != yaml.SequenceNode {
		return nil
	}
	return document.Content[0]
}
