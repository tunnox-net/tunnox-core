package conncode

import (
	"math/big"
	"strings"
	"testing"

	"tunnox-core/internal/cloud/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGenerator(t *testing.T) {
	tests := []struct {
		name   string
		config *models.ConnectionCodeGenerator
		want   struct {
			segmentLength int
			segmentCount  int
			separator     string
		}
	}{
		{
			name:   "nil config uses default",
			config: nil,
			want: struct {
				segmentLength int
				segmentCount  int
				separator     string
			}{3, 3, "-"},
		},
		{
			name:   "default config",
			config: models.DefaultConnectionCodeGenerator(),
			want: struct {
				segmentLength int
				segmentCount  int
				separator     string
			}{3, 3, "-"},
		},
		{
			name: "custom config",
			config: &models.ConnectionCodeGenerator{
				SegmentLength: 4,
				SegmentCount:  2,
				Separator:     "_",
				Charset:       "ABC123",
			},
			want: struct {
				segmentLength int
				segmentCount  int
				separator     string
			}{4, 2, "_"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGenerator(tt.config)
			require.NotNil(t, g)

			config := g.GetConfig()
			assert.Equal(t, tt.want.segmentLength, config.SegmentLength)
			assert.Equal(t, tt.want.segmentCount, config.SegmentCount)
			assert.Equal(t, tt.want.separator, config.Separator)
		})
	}
}

func TestGenerator_Generate(t *testing.T) {
	tests := []struct {
		name          string
		config        *models.ConnectionCodeGenerator
		expectFormat  string // regex-like description
		expectLen     int    // total length including separators
		segmentLen    int
		segmentCount  int
		separator     string
	}{
		{
			name:         "default format (abc-def-123)",
			config:       models.DefaultConnectionCodeGenerator(),
			segmentLen:   3,
			segmentCount: 3,
			separator:    "-",
			expectLen:    11, // 3+1+3+1+3 = 11
		},
		{
			name: "custom format (ABCD_EFGH)",
			config: &models.ConnectionCodeGenerator{
				SegmentLength: 4,
				SegmentCount:  2,
				Separator:     "_",
				Charset:       "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
			},
			segmentLen:   4,
			segmentCount: 2,
			separator:    "_",
			expectLen:    9, // 4+1+4 = 9
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGenerator(tt.config)

			// Generate multiple codes to test randomness
			codes := make(map[string]bool)
			for i := 0; i < 100; i++ {
				code, err := g.Generate()
				require.NoError(t, err)
				assert.Len(t, code, tt.expectLen)

				// Check format
				parts := strings.Split(code, tt.separator)
				assert.Equal(t, tt.segmentCount, len(parts))
				for _, part := range parts {
					assert.Len(t, part, tt.segmentLen)
					// Check all characters are in charset
					for _, ch := range part {
						assert.True(t, strings.ContainsRune(tt.config.Charset, ch),
							"character %c not in charset", ch)
					}
				}

				codes[code] = true
			}

			// Check randomness - should generate many unique codes
			assert.Greater(t, len(codes), 90, "should generate mostly unique codes")
		})
	}
}

func TestGenerator_Generate_EmptyCharset(t *testing.T) {
	g := NewGenerator(&models.ConnectionCodeGenerator{
		SegmentLength: 3,
		SegmentCount:  3,
		Separator:     "-",
		Charset:       "", // empty charset
	})

	_, err := g.Generate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "charset is empty")
}

func TestGenerator_GenerateUnique(t *testing.T) {
	g := NewGenerator(models.DefaultConnectionCodeGenerator())

	t.Run("generates unique code when none exist", func(t *testing.T) {
		code, err := g.GenerateUnique(func(c string) (bool, error) {
			return false, nil // nothing exists
		})
		require.NoError(t, err)
		assert.NotEmpty(t, code)
	})

	t.Run("retries on collision", func(t *testing.T) {
		callCount := 0
		code, err := g.GenerateUnique(func(c string) (bool, error) {
			callCount++
			if callCount < 5 {
				return true, nil // simulate collision
			}
			return false, nil // success on 5th try
		})
		require.NoError(t, err)
		assert.NotEmpty(t, code)
		assert.Equal(t, 5, callCount)
	})

	t.Run("fails after max attempts", func(t *testing.T) {
		_, err := g.GenerateUnique(func(c string) (bool, error) {
			return true, nil // always exists
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "100 attempts")
	})

	t.Run("propagates check error", func(t *testing.T) {
		_, err := g.GenerateUnique(func(c string) (bool, error) {
			return false, assert.AnError
		})
		assert.Error(t, err)
	})
}

func TestGenerator_Validate(t *testing.T) {
	g := NewGenerator(models.DefaultConnectionCodeGenerator())
	charset := models.DefaultConnectionCodeGenerator().Charset

	tests := []struct {
		name    string
		code    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid code",
			code:    "abc-def-123",
			wantErr: false,
		},
		{
			name:    "valid code with all valid chars",
			code:    string(charset[0:3]) + "-" + string(charset[3:6]) + "-" + string(charset[6:9]),
			wantErr: false,
		},
		{
			name:    "wrong segment count - too few",
			code:    "abc-def",
			wantErr: true,
			errMsg:  "expected 3 segments",
		},
		{
			name:    "wrong segment count - too many",
			code:    "abc-def-123-456",
			wantErr: true,
			errMsg:  "expected 3 segments",
		},
		{
			name:    "wrong segment length",
			code:    "abcd-def-123",
			wantErr: true,
			errMsg:  "expected length 3",
		},
		{
			name:    "invalid character - uppercase not in charset",
			code:    "ABC-def-123",
			wantErr: true,
			errMsg:  "invalid character",
		},
		{
			name:    "invalid character - special char",
			code:    "ab!-def-123",
			wantErr: true,
			errMsg:  "invalid character",
		},
		{
			name:    "empty code",
			code:    "",
			wantErr: true,
			errMsg:  "expected 3 segments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := g.Validate(tt.code)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGenerator_Validate_CustomConfig(t *testing.T) {
	g := NewGenerator(&models.ConnectionCodeGenerator{
		SegmentLength: 4,
		SegmentCount:  2,
		Separator:     "_",
		Charset:       "ABCD",
	})

	tests := []struct {
		name    string
		code    string
		wantErr bool
	}{
		{
			name:    "valid custom format",
			code:    "ABCD_DCBA",
			wantErr: false,
		},
		{
			name:    "wrong separator",
			code:    "ABCD-DCBA",
			wantErr: true,
		},
		{
			name:    "lowercase not in charset",
			code:    "abcd_dcba",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := g.Validate(tt.code)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGenerator_CalculateEntropy(t *testing.T) {
	tests := []struct {
		name           string
		config         *models.ConnectionCodeGenerator
		expectedMinLog int // minimum expected log10 of entropy
	}{
		{
			name:           "default config has high entropy",
			config:         models.DefaultConnectionCodeGenerator(),
			expectedMinLog: 10, // 33^9 = 4.6e13, log10 = ~13.6
		},
		{
			name: "small config has lower entropy",
			config: &models.ConnectionCodeGenerator{
				SegmentLength: 2,
				SegmentCount:  2,
				Separator:     "-",
				Charset:       "0123456789", // 10 chars
			},
			expectedMinLog: 3, // 10^4 = 10000
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGenerator(tt.config)
			entropy := g.CalculateEntropy()

			// Check entropy is positive
			assert.True(t, entropy.Cmp(big.NewInt(0)) > 0)

			// Check approximate magnitude
			// Calculate log10 by comparing with 10^expectedMinLog
			minEntropy := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(tt.expectedMinLog)), nil)
			assert.True(t, entropy.Cmp(minEntropy) >= 0,
				"entropy %s should be at least 10^%d", entropy.String(), tt.expectedMinLog)
		})
	}
}

func TestGenerator_GetEntropyString(t *testing.T) {
	g := NewGenerator(models.DefaultConnectionCodeGenerator())
	entropyStr := g.GetEntropyString()

	// Should contain scientific notation for large numbers
	assert.NotEmpty(t, entropyStr)
	// Default config has entropy > 10^12, so should use scientific notation
	assert.Contains(t, entropyStr, "10^")
}

func TestGenerator_GetEntropyString_SmallEntropy(t *testing.T) {
	g := NewGenerator(&models.ConnectionCodeGenerator{
		SegmentLength: 1,
		SegmentCount:  2,
		Separator:     "-",
		Charset:       "AB", // 2^2 = 4
	})
	entropyStr := g.GetEntropyString()

	assert.NotEmpty(t, entropyStr)
	assert.Equal(t, "4", entropyStr) // Should be simple number
}

func TestGenerator_GetConfig(t *testing.T) {
	config := &models.ConnectionCodeGenerator{
		SegmentLength: 5,
		SegmentCount:  4,
		Separator:     ".",
		Charset:       "XYZ",
	}
	g := NewGenerator(config)

	got := g.GetConfig()
	assert.Equal(t, config.SegmentLength, got.SegmentLength)
	assert.Equal(t, config.SegmentCount, got.SegmentCount)
	assert.Equal(t, config.Separator, got.Separator)
	assert.Equal(t, config.Charset, got.Charset)
}
