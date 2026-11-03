package openai

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
	Model     string            `json:"model"`
	Input     string            `json:"input"`
	Questions []requestQuestion `json:"questions"`
}

type requestQuestion struct {
	Type         string `json:"type"`
	Name         string `json:"name"`
	Instructions string `json:"instructions"`
}

type responseBody struct {
	Model   string           `json:"model"`
	Answers []responseAnswer `json:"answers"`
}

type responseAnswer struct {
	Type        string  `json:"type"`
	Name        string  `json:"name"`
	Probability float64 `json:"probability"`
}

func init() {
	backend.Register(implementation{})
}

func (implementation) Name() string {
	return "openai"
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
	body, err := makeRequest(model, state, questions)
	if err != nil {
		return backend.Result{}, err
	}

	timer := backend.NewConnTimer()
	result := backend.Result{}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/decisions", bytes.NewReader(body))
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
	return decodeResponse(resp, result)
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

func makeRequest(model string, state map[string]any, questions []backend.Question) ([]byte, error) {
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("encode state: %w", err)
	}
	requestQuestions := make([]requestQuestion, len(questions))
	for index, question := range questions {
		instructions := question.Text
		if question.Not != "" {
			instructions += "\nAnswer false when: " + question.Not
		}
		requestQuestions[index] = requestQuestion{
			Type:         "predicate",
			Name:         question.ID,
			Instructions: instructions,
		}
	}
	request := requestBody{
		Model:     model,
		Input:     string(stateJSON),
		Questions: requestQuestions,
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	return body, nil
}

func decodeResponse(resp *http.Response, result backend.Result) (backend.Result, error) {
	var decoded responseBody
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return result, fmt.Errorf("decode response: %w", err)
	}
	result.Model = decoded.Model
	result.P = make(map[string]float64, len(decoded.Answers))
	for _, answer := range decoded.Answers {
		if answer.Type == "predicate" {
			result.P[answer.Name] = min(1, max(0, answer.Probability))
		}
	}
	return result, nil
}
