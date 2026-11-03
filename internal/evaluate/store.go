package evaluate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chilogen/jit-rules/internal/fileutil"
	"github.com/chilogen/jit-rules/internal/paths"
	"github.com/chilogen/jit-rules/internal/unit"
)

func saveTest(u *unit.Unit, cases []CaseResult, now time.Time) error {
	record := TestRecord{Hash: u.Hash, TS: now, Cases: cases}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode test result: %w", err)
	}
	path := testPath(u.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create test result directory: %w", err)
	}
	if err := fileutil.WriteAtomic(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write test result: %w", err)
	}
	return nil
}

func loadTest(id string) (*TestRecord, error) {
	data, err := os.ReadFile(testPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read test result: %w", err)
	}
	var record TestRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("parse test result: %w", err)
	}
	return &record, nil
}

func testPath(id string) string {
	name := strings.ReplaceAll(id, "/", "__") + ".json"
	return filepath.Join(paths.State(), "tests", name)
}
