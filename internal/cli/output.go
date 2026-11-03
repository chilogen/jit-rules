package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

type envelope struct {
	OK       bool     `json:"ok"`
	Data     any      `json:"data"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

// Output writes either the stable JSON envelope or human-readable output.
func Output(cmd *cobra.Command, ok bool, data any, human string, errs []error, warns []string) {
	if jsonMode {
		payload := envelope{
			OK:       ok,
			Data:     data,
			Errors:   errorStrings(errs),
			Warnings: nonNilStrings(warns),
		}
		_ = json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
		return
	}
	if human != "" {
		fmt.Fprintln(cmd.OutOrStdout(), human)
	}
	for _, err := range errs {
		fmt.Fprintln(cmd.ErrOrStderr(), err)
	}
	for _, warning := range warns {
		fmt.Fprintln(cmd.ErrOrStderr(), warning)
	}
}

func errorStrings(errs []error) []string {
	result := make([]string, 0, len(errs))
	for _, err := range errs {
		result = append(result, err.Error())
	}
	return result
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
