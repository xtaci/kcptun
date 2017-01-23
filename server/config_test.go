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

func TestParseJSONConfigSuccess(t *testing.T) {
	path := writeTempConfig(t, `{"listen":"0.0.0.0:29900","target":"127.0.0.1:4000","key":"secret","mtu":1350,"acknodelay":true,"closewait":17}`)

	var cfg Config
	if err := parseJSONConfig(&cfg, path); err != nil {
		t.Fatalf("parseJSONConfig returned error: %v", err)
	}

	if cfg.Listen != "0.0.0.0:29900" || cfg.Target != "127.0.0.1:4000" {
		t.Fatalf("unexpected addresses: %+v", cfg)
	}

	if cfg.Key != "secret" {
		t.Fatalf("expected key to be populated")
	}

	if cfg.MTU != 1350 || !cfg.AckNodelay || cfg.CloseWait != 17 {
		t.Fatalf("unexpected numeric or boolean fields: %+v", cfg)
	}
}

func TestParseJSONConfigMissingFile(t *testing.T) {
	var cfg Config
	missing := filepath.Join(t.TempDir(), "missing.json")
	if err := parseJSONConfig(&cfg, missing); err == nil {
		t.Fatalf("parseJSONConfig expected error for missing file")
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	return path
}
