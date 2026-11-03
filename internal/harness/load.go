package harness

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/chilogen/jit-rules/assets"
	"github.com/chilogen/jit-rules/internal/paths"
	"gopkg.in/yaml.v3"
)

func loadAll() ([]*Definition, error) {
	builtin, err := fs.Glob(assets.FS, "harnesses/*.yaml")
	if err != nil {
		return nil, err
	}
	defs := make([]*Definition, 0, len(builtin))
	index := make(map[string]int)
	for _, name := range builtin {
		data, readErr := assets.FS.ReadFile(name)
		if readErr != nil {
			return nil, readErr
		}
		if err := addDefinition(&defs, index, name, data); err != nil {
			return nil, err
		}
	}
	user, err := filepath.Glob(filepath.Join(paths.Config(), "harnesses", "*.yaml"))
	if err != nil {
		return nil, err
	}
	for _, name := range user {
		data, readErr := readDefinitionFile(name)
		if readErr != nil {
			return nil, readErr
		}
		if err := addDefinition(&defs, index, name, data); err != nil {
			return nil, err
		}
	}
	return defs, nil
}

func readDefinitionFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func addDefinition(defs *[]*Definition, index map[string]int, name string, data []byte) error {
	def, err := parseDefinition(data)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if i, ok := index[def.Name]; ok {
		(*defs)[i] = def
		return nil
	}
	index[def.Name] = len(*defs)
	*defs = append(*defs, def)
	return nil
}

func parseDefinition(data []byte) (*Definition, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var def Definition
	if err := decoder.Decode(&def); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple YAML documents")
		}
		return nil, err
	}
	return &def, nil
}

func find(name string) (*Definition, error) {
	defs, err := loadAll()
	if err != nil {
		return nil, err
	}
	for _, def := range defs {
		if def.Name == name {
			return def, nil
		}
	}
	return nil, fmt.Errorf("unknown harness %q", name)
}

func detectDef(defs []*Definition, raw map[string]any) *Definition {
	ordered := append([]*Definition(nil), defs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Detect.Order < ordered[j].Detect.Order
	})
	for _, def := range ordered {
		if detectionMatches(def.Detect, raw) {
			return def
		}
	}
	return nil
}

func detectionMatches(d Detect, raw map[string]any) bool {
	if d.Fallback {
		return true
	}
	anyMatch := len(d.AnyFields) == 0
	for _, field := range d.AnyFields {
		if _, ok := raw[field]; ok {
			anyMatch = true
			break
		}
	}
	if !anyMatch {
		return false
	}
	for field, expected := range d.FieldEquals {
		value, ok := raw[field]
		if !ok || fmt.Sprint(value) != fmt.Sprint(expected) {
			return false
		}
	}
	return len(d.AnyFields) > 0 || len(d.FieldEquals) > 0
}
