package archtest

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/chilogen/jit-rules"

var forbiddenLiterals = map[string]struct{}{
	"jev": {}, "openai": {}, "codex": {}, "trae": {},
	"deepseek": {}, "claude-code": {},
}

type listedPackage struct {
	ImportPath string
	Imports    []string
}

func TestDependencyBoundaries(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	root := repositoryRoot(t)
	packages, err := listPackages(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, violation := range dependencyViolations(packages) {
		t.Error(violation)
	}
}

func TestForbiddenLiterals(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	violations, err := literalViolations(filepath.Join(repositoryRoot(t), "internal"))
	if err != nil {
		t.Fatal(err)
	}
	for _, violation := range violations {
		t.Error(violation)
	}
}

func TestForbiddenLiteralsDetectViolation(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	root := t.TempDir()
	source := []byte("package sample\n\nconst backend = \"codex\"\n")
	if err := os.WriteFile(filepath.Join(root, "sample.go"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	violations, err := literalViolations(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1", len(violations))
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate architecture test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func listPackages(root string) ([]listedPackage, error) {
	command := exec.Command("go", "list", "-deps", "-json", "./...")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	var packages []listedPackage
	for decoder.More() {
		var pkg listedPackage
		if err := decoder.Decode(&pkg); err != nil {
			return nil, fmt.Errorf("decode go list output: %w", err)
		}
		packages = append(packages, pkg)
	}
	return packages, nil
}

func dependencyViolations(packages []listedPackage) []string {
	var violations []string
	for _, pkg := range packages {
		sourceKind, sourceConcrete := concreteImplementation(pkg.ImportPath)
		for _, imported := range pkg.Imports {
			targetKind, targetConcrete := concreteImplementation(imported)
			if !targetConcrete {
				continue
			}
			if sourceConcrete && (sourceKind != targetKind || pkg.ImportPath != imported) {
				violations = append(violations,
					fmt.Sprintf("%s imports concrete implementation %s", pkg.ImportPath, imported))
				continue
			}
			if !allowedConcreteImporter(pkg.ImportPath) {
				violations = append(violations,
					fmt.Sprintf("%s imports concrete implementation %s", pkg.ImportPath, imported))
			}
		}
	}
	return violations
}

func concreteImplementation(importPath string) (string, bool) {
	relative := strings.TrimPrefix(importPath, modulePath+"/")
	parts := strings.Split(relative, "/")
	if len(parts) < 3 || parts[0] != "internal" {
		return "", false
	}
	switch parts[1] {
	case "backend", "installer", "launcher":
		return parts[1], true
	default:
		return "", false
	}
}

func allowedConcreteImporter(importPath string) bool {
	relative := strings.TrimPrefix(importPath, modulePath+"/")
	return relative == "internal/plugins" ||
		relative == "internal/cli" ||
		strings.HasPrefix(relative, "internal/cli/") ||
		strings.HasPrefix(relative, "cmd/")
}

func literalViolations(root string) ([]string, error) {
	var violations []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if excludedLiteralDirectory(root, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		found, err := fileLiteralViolations(path)
		if err != nil {
			return err
		}
		violations = append(violations, found...)
		return nil
	})
	return violations, err
}

func excludedLiteralDirectory(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." {
		return false
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if parts[0] == "archtest" {
		return true
	}
	return len(parts) >= 2 &&
		(parts[0] == "backend" || parts[0] == "installer" || parts[0] == "launcher")
}

func fileLiteralViolations(path string) ([]string, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var violations []string
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err == nil {
			if _, forbidden := forbiddenLiterals[value]; forbidden {
				position := fileSet.Position(literal.Pos())
				violations = append(violations,
					fmt.Sprintf("%s:%d: forbidden string literal %q", path, position.Line, value))
			}
		}
		return true
	})
	return violations, nil
}
