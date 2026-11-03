package profile

import "testing"

func TestDefaults(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	got := Defaults()
	if got.Judge.Backend != "jev" || got.Judge.TimeoutMs != 3500 ||
		got.Judge.MaxFieldChars != 2000 || !got.Judge.SendContent {
		t.Fatalf("unexpected judge defaults: %#v", got.Judge)
	}
	assertBackendValue(t, got, "jev", "model", "jev-latest")
	assertBackendValue(t, got, "jev", "base_url", "https://api.typesafe.ai")
	assertBackendValue(t, got, "jev", "api_key_env", "TYPESAFE_API_KEY")
	assertBackendValue(t, got, "jev", "threshold", 0.3)
	assertBackendValue(t, got, "openai", "model", "gpt-6-luna")
	assertBackendValue(t, got, "openai", "base_url", "https://api.openai.com/v1")
	assertBackendValue(t, got, "openai", "api_key_env", "OPENAI_API_KEY")
	assertBackendValue(t, got, "openai", "threshold", 0.5)
	if got.RedeliverBytes != 80000 || got.DeadlineMs != 5000 || got.MaxInjectChars != 4000 {
		t.Fatalf("unexpected delivery defaults: %#v", got)
	}
	if got.Journal.KeepDays != 30 || !got.Journal.StoreContent {
		t.Fatalf("unexpected journal defaults: %#v", got.Journal)
	}
}

func TestThreshold(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	settings := Defaults()
	if got := settings.Threshold(); got != 0.3 {
		t.Fatalf("default Threshold() = %v", got)
	}
	settings.Judge.Backend = "openai"
	if got := settings.Threshold(); got != 0.5 {
		t.Fatalf("configured Threshold() = %v", got)
	}
	settings.Judge.Backend = "missing"
	if got := settings.Threshold(); got != 0.5 {
		t.Fatalf("missing Threshold() = %v", got)
	}
	settings.Backends["integer"] = map[string]any{"threshold": 1}
	settings.Judge.Backend = "integer"
	if got := settings.Threshold(); got != 1 {
		t.Fatalf("integer Threshold() = %v", got)
	}
}

func assertBackendValue(t *testing.T, settings Settings, backend, key string, want any) {
	t.Helper()
	if got := settings.Backends[backend][key]; got != want {
		t.Errorf("backends.%s.%s = %#v, want %#v", backend, key, got, want)
	}
}
