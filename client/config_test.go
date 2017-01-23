// The MIT License (MIT)
//
// Copyright (c) 2016 xtaci
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

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

func TestValidateConnCount(t *testing.T) {
	tests := []struct {
		name  string
		count int
		valid bool
	}{
		{name: "minimum", count: 1, valid: true},
		{name: "maximum", count: maxConnCount, valid: true},
		{name: "zero", count: 0},
		{name: "negative", count: -1},
		{name: "above maximum", count: maxConnCount + 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConnCount(tt.count)
			if tt.valid && err != nil {
				t.Fatalf("validateConnCount(%d) returned error: %v", tt.count, err)
			}
			if !tt.valid && err == nil {
				t.Fatalf("validateConnCount(%d) expected error", tt.count)
			}
		})
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
