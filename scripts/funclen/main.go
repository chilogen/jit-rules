package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const maximumFunctionLines = 60

func main() {
	targets := os.Args[1:]
	if len(targets) == 0 {
		targets = []string{"."}
	}
	var violations []string
	for _, target := range targets {
		found, err := checkFunctions(target)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		violations = append(violations, found...)
	}
	for _, violation := range violations {
		fmt.Println(violation)
	}
	if len(violations) > 0 {
		os.Exit(1)
	}
}

func checkFunctions(root string) ([]string, error) {
	var violations []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		found, err := checkFile(path)
		if err != nil {
			return err
		}
		violations = append(violations, found...)
		return nil
	})
	return violations, err
}

func checkFile(path string) ([]string, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	var violations []string
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		start := fileSet.Position(function.Pos()).Line
		end := fileSet.Position(function.End()).Line
		if lines := end - start + 1; lines > maximumFunctionLines {
			violations = append(violations,
				fmt.Sprintf("%s:%d: %s is %d lines (maximum %d)",
					path, start, function.Name.Name, lines, maximumFunctionLines))
		}
	}
	return violations, nil
}
