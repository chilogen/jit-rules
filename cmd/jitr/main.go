package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/chilogen/jit-rules/internal/cli"
	"github.com/chilogen/jit-rules/internal/journal"
	_ "github.com/chilogen/jit-rules/internal/plugins"
)

func main() {
	defer recoverMain()
	os.Exit(cli.Execute())
}

func recoverMain() {
	recovered := recover()
	if recovered == nil {
		return
	}
	err := errors.New(fmt.Sprint(recovered))
	if len(os.Args) > 1 && os.Args[1] == "hook" {
		logPanic(err)
		os.Exit(0)
	}
	fmt.Fprintf(os.Stderr, "panic: %v\n", err)
	os.Exit(3)
}

func logPanic(err error) {
	defer func() {
		_ = recover()
	}()
	journal.LogError("panic", err)
}
