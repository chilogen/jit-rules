//go:build live

package jev

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/chilogen/jit-rules/internal/backend"
)

func TestLiveCoreAuthorExamples(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	if os.Getenv("TYPESAFE_API_KEY") == "" {
		t.Skip("TYPESAFE_API_KEY is not set")
	}
	config := map[string]any{
		"model":       "jev-latest",
		"base_url":    "https://api.typesafe.ai",
		"api_key_env": "TYPESAFE_API_KEY",
	}
	question := backend.Question{
		ID:   "core/author",
		Text: "Is the user asking to add, change, remove, or review a rule, habit, or preference that the agent should follow in future work?",
		Not:  "The user only asks for a one-off change to the current task.",
	}
	examples := []struct {
		name   string
		prompt string
		hit    bool
	}{
		{name: "chinese-hit", prompt: "以后写 commit message 都用中文", hit: true},
		{name: "english-hit", prompt: "Remember to always run the tests before committing.", hit: true},
		{name: "miss", prompt: "把这个函数改成异步的", hit: false},
	}
	for _, example := range examples {
		t.Run(example.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			result, err := (implementation{}).Judge(
				ctx,
				map[string]any{"kind": "prompt", "prompt": example.prompt},
				[]backend.Question{question},
				config,
			)
			if err != nil {
				t.Fatalf("Judge() error = %v", err)
			}
			probability := result.P[question.ID]
			if example.hit && probability < 0.3 {
				t.Fatalf("P = %v, want at least 0.3", probability)
			}
			if !example.hit && probability >= 0.3 {
				t.Fatalf("P = %v, want below 0.3", probability)
			}
		})
	}
}
