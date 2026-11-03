package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chilogen/jit-rules/internal/backend"
)

func TestJudgeRequestAndResponse(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	t.Setenv("OPENAI_TEST_KEY", "secret")
	state := map[string]any{"kind": "tool"}
	questions := []backend.Question{
		{ID: "a", Text: "is a", Not: "not a"},
		{ID: "b", Text: "is b"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/decisions" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if want := expectedRequest(); !reflect.DeepEqual(got, want) {
			t.Errorf("body = %#v, want %#v", got, want)
		}
		io.WriteString(w, `{"model":"actual-model","answers":[{"type":"predicate","name":"a","probability":1.2},{"type":"predicate","name":"b","probability":-0.2},{"type":"refusal","name":"c"}]}`)
	}))
	defer server.Close()

	result, err := (implementation{}).Judge(context.Background(), state, questions, testConfig(server.URL))
	if err != nil {
		t.Fatalf("Judge() error = %v", err)
	}
	if result.Model != "actual-model" || result.P["a"] != 1 || result.P["b"] != 0 {
		t.Fatalf("Judge() = %#v", result)
	}
}

func TestJudgeFailures(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	t.Setenv("OPENAI_TEST_KEY", "secret")
	tests := []struct {
		name    string
		handler http.HandlerFunc
		timeout time.Duration
	}{
		{
			name: "status",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "bad", http.StatusInternalServerError)
			},
		},
		{
			name: "invalid-json",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				io.WriteString(w, "{")
			},
		},
		{
			name: "timeout",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(30 * time.Millisecond)
				io.WriteString(w, `{}`)
			},
			timeout: 5 * time.Millisecond,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(test.handler)
			defer server.Close()
			ctx := context.Background()
			if test.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, test.timeout)
				defer cancel()
			}
			_, err := (implementation{}).Judge(ctx, nil, nil, testConfig(server.URL))
			if err == nil {
				t.Fatal("Judge() error = nil")
			}
			if errors.Is(err, backend.ErrNotConfigured) {
				t.Fatalf("Judge() error wraps ErrNotConfigured: %v", err)
			}
		})
	}
}

func TestJudgeMissingAPIKeyDoesNotRequest(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	t.Setenv("OPENAI_TEST_KEY", "")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()

	_, err := (implementation{}).Judge(context.Background(), nil, nil, testConfig(server.URL))
	if !errors.Is(err, backend.ErrNotConfigured) {
		t.Fatalf("Judge() error = %v, want ErrNotConfigured", err)
	}
	if !strings.Contains(err.Error(), "OPENAI_TEST_KEY") {
		t.Fatalf("Judge() error = %q, want variable name", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("requests = %d, want 0", got)
	}
}

func TestJudgeFailureResultsCarryConnectionTime(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	t.Setenv("OPENAI_TEST_KEY", "secret")
	original := http.DefaultClient
	defer func() { http.DefaultClient = original }()

	tests := []struct {
		name      string
		transport timedTransport
	}{
		{name: "request", transport: timedTransport{err: errors.New("network")}},
		{name: "status", transport: timedTransport{status: http.StatusInternalServerError, body: `{}`}},
		{name: "decode", transport: timedTransport{status: http.StatusOK, body: `{`}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			http.DefaultClient = &http.Client{Transport: test.transport}
			result, err := (implementation{}).Judge(context.Background(), nil, nil, testConfig("http://example.test"))
			if err == nil {
				t.Fatal("Judge() error = nil")
			}
			if result.ConnMs < 20 {
				t.Fatalf("ConnMs = %d, want at least 20", result.ConnMs)
			}
		})
	}
}

func expectedRequest() map[string]any {
	return map[string]any{
		"model": "test-model",
		"input": `{"kind":"tool"}`,
		"questions": []any{
			map[string]any{
				"type":         "predicate",
				"name":         "a",
				"instructions": "is a\nAnswer false when: not a",
			},
			map[string]any{
				"type":         "predicate",
				"name":         "b",
				"instructions": "is b",
			},
		},
	}
}

type timedTransport struct {
	status int
	body   string
	err    error
}

func (transport timedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	time.Sleep(20 * time.Millisecond)
	httptrace.ContextClientTrace(req.Context()).GotConn(httptrace.GotConnInfo{})
	if transport.err != nil {
		return nil, transport.err
	}
	return &http.Response{
		StatusCode: transport.status,
		Status:     http.StatusText(transport.status),
		Body:       io.NopCloser(strings.NewReader(transport.body)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func testConfig(baseURL string) map[string]any {
	return map[string]any{
		"model":       "test-model",
		"base_url":    baseURL,
		"api_key_env": "OPENAI_TEST_KEY",
	}
}
