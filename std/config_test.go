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

package std

import (
	"encoding/json"
	"os"
	"testing"
)

func TestBaseConfigApplyMode(t *testing.T) {
	tests := []struct {
		mode         string
		wantNoDelay  int
		wantInterval int
		wantResend   int
		wantNC       int
		wantFound    bool
	}{
		{"normal", 0, 40, 2, 1, true},
		{"fast", 0, 30, 2, 1, true},
		{"fast2", 1, 20, 2, 1, true},
		{"fast3", 1, 10, 2, 1, true},
		{"manual", 0, 0, 0, 0, false},  // unknown mode
		{"invalid", 0, 0, 0, 0, false}, // unknown mode
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			cfg := &BaseConfig{Mode: tt.mode}
			found := cfg.ApplyMode()

			if found != tt.wantFound {
				t.Errorf("ApplyMode() returned %v, want %v", found, tt.wantFound)
			}

			if tt.wantFound {
				if cfg.NoDelay != tt.wantNoDelay {
					t.Errorf("NoDelay = %v, want %v", cfg.NoDelay, tt.wantNoDelay)
				}
				if cfg.Interval != tt.wantInterval {
					t.Errorf("Interval = %v, want %v", cfg.Interval, tt.wantInterval)
				}
				if cfg.Resend != tt.wantResend {
					t.Errorf("Resend = %v, want %v", cfg.Resend, tt.wantResend)
				}
				if cfg.NoCongestion != tt.wantNC {
					t.Errorf("NoCongestion = %v, want %v", cfg.NoCongestion, tt.wantNC)
				}
			}
		})
	}
}

func TestParseJSONConfig(t *testing.T) {
	// Create temp config file
	tmpfile, err := os.CreateTemp("", "config*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	testConfig := &BaseConfig{
		Key:       "testkey",
		Crypt:     "aes",
		Mode:      "fast",
		MTU:       1350,
		SndWnd:    128,
		RcvWnd:    512,
		KeepAlive: 10,
	}

	encoder := json.NewEncoder(tmpfile)
	if err := encoder.Encode(testConfig); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	// Test parsing
	loadedConfig := &BaseConfig{}
	if err := ParseJSONConfig(loadedConfig, tmpfile.Name()); err != nil {
		t.Fatalf("ParseJSONConfig failed: %v", err)
	}

	if loadedConfig.Key != testConfig.Key {
		t.Errorf("Key = %v, want %v", loadedConfig.Key, testConfig.Key)
	}
	if loadedConfig.Crypt != testConfig.Crypt {
		t.Errorf("Crypt = %v, want %v", loadedConfig.Crypt, testConfig.Crypt)
	}
	if loadedConfig.MTU != testConfig.MTU {
		t.Errorf("MTU = %v, want %v", loadedConfig.MTU, testConfig.MTU)
	}
}

func TestParseJSONConfigMissingFile(t *testing.T) {
	cfg := &BaseConfig{}
	err := ParseJSONConfig(cfg, "/nonexistent/path/config.json")
	if err == nil {
		t.Error("ParseJSONConfig should fail for missing file")
	}
}
