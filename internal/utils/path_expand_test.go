package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		wantErr  bool
		checkFn  func(string) bool
	}{
		{
			name:    "expand ~/path",
			input:   "~/test.log",
			wantErr: false,
			checkFn: func(got string) bool {
				expected := filepath.Join(homeDir, "test.log")
				return got == expected
			},
		},
		{
			name:    "expand ~ only",
			input:   "~",
			wantErr: false,
			checkFn: func(got string) bool {
				return got == homeDir
			},
		},
		{
			name:    "relative path",
			input:   "./test.log",
			wantErr: false,
			checkFn: func(got string) bool {
				return filepath.IsAbs(got) && filepath.Base(got) == "test.log"
			},
		},
		{
			name:    "absolute path",
			input:   "/tmp/test.log",
			wantErr: false,
			checkFn: func(got string) bool {
				return got == "/tmp/test.log"
			},
		},
		{
			name:    "empty path",
			input:   "",
			wantErr: true,
			checkFn: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checkFn != nil {
				if !tt.checkFn(got) {
					t.Errorf("ExpandPath() = %v, check failed", got)
				}
			}
		})
	}
}

