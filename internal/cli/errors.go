package cli

import (
	"errors"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
)

type internalError struct {
	err error
}

func (e internalError) Error() string { return e.err.Error() }
func (e internalError) Unwrap() error { return e.err }

type silentError struct {
	code int
}

func (e silentError) Error() string { return "silent command result" }

// Internal marks an error as an environment or implementation failure.
func Internal(err error) error {
	return internalError{err: err}
}

// Silent returns an exit result that Execute must not print.
func Silent(code int) error {
	return silentError{code: code}
}

func silentCode(err error) (int, bool) {
	var target silentError
	if errors.As(err, &target) {
		return target.code, true
	}
	return 0, false
}

func errorCode(err error) int {
	var marked internalError
	if errors.As(err, &marked) {
		return 3
	}
	if errors.Is(err, fs.ErrNotExist) {
		return 1
	}
	if isSystemError(err) {
		return 3
	}
	return 1
}

func isSystemError(err error) bool {
	targets := []any{
		new(*fs.PathError),
		new(*os.LinkError),
		new(*os.SyscallError),
		new(*exec.Error),
		new(*exec.ExitError),
		new(*url.Error),
	}
	for _, target := range targets {
		if errors.As(err, target) {
			return true
		}
	}
	return false
}
