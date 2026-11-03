package journal

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chilogen/jit-rules/internal/paths"
)

func appendRecord(record Record, storeContent bool) error {
	if !storeContent {
		record.Event.Content = hash(record.Event.Content)
		record.Event.Shell = hash(record.Event.Shell)
		record.Event.Prompt = hash(record.Event.Prompt)
	}
	return appendJSON(journalPath(record.TS), record)
}

func appendJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func since(start time.Time) ([]Record, error) {
	files, err := filepath.Glob(filepath.Join(paths.State(), "journal", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	records := make([]Record, 0)
	for _, path := range files {
		if filepath.Base(path) < start.UTC().Format("2006-01-02")+".jsonl" {
			continue
		}
		day, err := readRecords(path, start)
		if err != nil {
			return nil, err
		}
		records = append(records, day...)
	}
	return records, nil
}

func readRecords(path string, start time.Time) ([]Record, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var records []Record
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var record Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, err
		}
		if !record.TS.Before(start) {
			records = append(records, record)
		}
	}
	return records, scanner.Err()
}

func gc(now time.Time, keepDays int) {
	files, _ := filepath.Glob(filepath.Join(paths.State(), "journal", "*.jsonl"))
	cutoff := now.UTC().AddDate(0, 0, -keepDays).Format("2006-01-02")
	for _, path := range files {
		day := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		if day < cutoff {
			_ = os.Remove(path)
		}
	}
}

func logError(stage string, err error) {
	if err == nil {
		return
	}
	entry := struct {
		TS    time.Time `json:"ts"`
		Stage string    `json:"stage"`
		Err   string    `json:"err"`
	}{TS: time.Now().UTC(), Stage: stage, Err: err.Error()}
	_ = appendJSON(filepath.Join(paths.State(), "errors.jsonl"), entry)
}

func journalPath(timestamp time.Time) string {
	return filepath.Join(paths.State(), "journal", timestamp.UTC().Format("2006-01-02")+".jsonl")
}

func hash(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}
