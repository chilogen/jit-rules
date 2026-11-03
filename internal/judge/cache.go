package judge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/chilogen/jit-rules/internal/backend"
	"github.com/chilogen/jit-rules/internal/fileutil"
	"github.com/chilogen/jit-rules/internal/paths"
)

type cacheValue struct {
	P     float64 `json:"p"`
	Model string  `json:"model"`
}

func cacheKey(backendName, model string, question backend.Question, stateJSON []byte) string {
	var input strings.Builder
	input.WriteString(backendName)
	input.WriteByte('\n')
	input.WriteString(model)
	input.WriteByte('\n')
	input.WriteString(question.Text)
	input.WriteByte('\n')
	input.WriteString(question.Not)
	input.WriteByte('\n')
	input.Write(stateJSON)
	sum := sha256.Sum256([]byte(input.String()))
	return hex.EncodeToString(sum[:])
}

func cached(key string) (cacheValue, bool) {
	data, err := os.ReadFile(cachePath(key))
	if err != nil {
		return cacheValue{}, false
	}
	var value cacheValue
	if json.Unmarshal(data, &value) != nil {
		return cacheValue{}, false
	}
	return value, true
}

func storeCached(key string, value cacheValue) {
	data, err := json.Marshal(value)
	if err == nil {
		_ = fileutil.WriteAtomic(cachePath(key), data, 0o600)
	}
}

func cachePath(key string) string {
	prefix := key
	if len(prefix) > 2 {
		prefix = prefix[:2]
	}
	return filepath.Join(paths.Cache(), "judge", prefix, key+".json")
}

func hash(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func marshalState(state map[string]any) []byte {
	data, _ := json.Marshal(state)
	return data
}
