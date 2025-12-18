package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseJSONConfigSuccessClient(t *testing.T) {
	path := writeTempClientConfig(t, `{"localaddr":"127.0.0.1:12948","remoteaddr":"2.2.2.2:4000","key":"secret","conn":2,"tcp":true,"closewait":9}`)

	var cfg Config
	if err := parseJSONConfig(&cfg, path); err != nil {
		t.Fatalf("parseJSONConfig returned error: %v", err)
	}

	if cfg.LocalAddr != "127.0.0.1:12948" || cfg.RemoteAddr != "2.2.2.2:4000" {
		t.Fatalf("unexpected addresses: %+v", cfg)
	}

	if cfg.Key != "secret" || cfg.Conn != 2 || !cfg.TCP || cfg.CloseWait != 9 {
		t.Fatalf("unexpected field values: %+v", cfg)
	}
}

func TestParseJSONConfigMissingFileClient(t *testing.T) {
	var cfg Config
	missing := filepath.Join(t.TempDir(), "missing.json")
	if err := parseJSONConfig(&cfg, missing); err == nil {
		t.Fatalf("parseJSONConfig expected error for missing file")
	}
}

func writeTempClientConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	return path
}
