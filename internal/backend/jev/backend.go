package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptrace"
	"os"
	"strings"

	"github.com/chilogen/jit-rules/internal/backend"
)

type implementation struct{}

type requestBody struct {
	Model     string                     `json:"model"`
	State     map[string]any             `json:"state"`
	Questions map[string]requestQuestion `json:"questions"`
}

type requestQuestion struct {
	Type         string           `json:"type"`
	Instructions string           `json:"instructions"`
	Criteria     *requestCriteria `json:"criteria,omitempty"`
}

type requestCriteria struct {
	True  string `json:"true"`
	False string `json:"false"`
}

type responseBody struct {
	Model   string                    `json:"model"`
	Answers map[string]responseAnswer `json:"answers"`
}

type responseAnswer struct {
	Noul float64 `json:"noul"`
}

func init() {
	backend.Register(implementation{})
}

func (implementation) Name() string {
	return "jev"
}

func (implementation) Judge(
	ctx context.Context,
	state map[string]any,
	questions []backend.Question,
	cfg map[string]any,
) (backend.Result, error) {
	model, baseURL, key, err := config(cfg)
	if err != nil {
		return backend.Result{}, err
	}
	body, err := json.Marshal(makeRequest(model, state, questions))
	if err != nil {
		return backend.Result{}, fmt.Errorf("encode request: %w", err)
	}

	timer := backend.NewConnTimer()
	result := backend.Result{}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		result.ConnMs = timer.Ms()
		return result, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), timer.Trace()))

	resp, err := http.DefaultClient.Do(req)
	result.ConnMs = timer.Ms()
	if err != nil {
		return result, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return result, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var decoded responseBody
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return result, fmt.Errorf("decode response: %w", err)
	}
	result.Model = decoded.Model
	if result.Model == "" {
		result.Model = model
	}
	result.P = make(map[string]float64, len(decoded.Answers))
	for id, answer := range decoded.Answers {
		result.P[id] = answer.Noul
	}
	return result, nil
}

func config(cfg map[string]any) (model, baseURL, key string, err error) {
	model, err = requiredString(cfg, "model")
	if err != nil {
		return "", "", "", err
	}
	baseURL, err = requiredString(cfg, "base_url")
	if err != nil {
		return "", "", "", err
	}
	keyEnv, err := requiredString(cfg, "api_key_env")
	if err != nil {
		return "", "", "", err
	}
	key = os.Getenv(keyEnv)
	if key == "" {
		return "", "", "", fmt.Errorf("%w: %s 未设置", backend.ErrNotConfigured, keyEnv)
	}
	return model, baseURL, key, nil
}

func requiredString(cfg map[string]any, name string) (string, error) {
	value, ok := cfg[name].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("missing backend config: %s", name)
	}
	return value, nil
}

func makeRequest(model string, state map[string]any, questions []backend.Question) requestBody {
	request := requestBody{
		Model:     model,
		State:     state,
		Questions: make(map[string]requestQuestion, len(questions)),
	}
	for _, question := range questions {
		item := requestQuestion{Type: "noul", Instructions: question.Text}
		if question.Not != "" {
			item.Criteria = &requestCriteria{True: question.Text, False: question.Not}
		}
		request.Questions[question.ID] = item
	}
	return request
}
