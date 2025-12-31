package services

import (
	"math/big"
	"strings"
	"testing"

	"tunnox-core/internal/cloud/models"
)

func TestConnectionCodeGenerator_Generate(t *testing.T) {
	generator := NewConnectionCodeGenerator(nil) // 使用默认配置

	code, err := generator.Generate()
	if err != nil {
		t.Fatalf("Failed to generate code: %v", err)
	}

	t.Logf("Generated code: %s", code)

	// 验证格式
	parts := strings.Split(code, "-")
	if len(parts) != 3 {
		t.Errorf("Expected 3 segments, got %d", len(parts))
	}

	for i, part := range parts {
		if len(part) != 3 {
			t.Errorf("Segment %d has length %d, expected 3", i, len(part))
		}
	}
}

func TestConnectionCodeGenerator_GenerateMultiple(t *testing.T) {
	generator := NewConnectionCodeGenerator(nil)

	codes := make(map[string]bool)
	count := 1000

	for i := 0; i < count; i++ {
		code, err := generator.Generate()
		if err != nil {
			t.Fatalf("Failed to generate code at iteration %d: %v", i, err)
		}

		if codes[code] {
			t.Errorf("Duplicate code generated: %s", code)
		}
		codes[code] = true
	}

	t.Logf("Successfully generated %d unique codes", len(codes))
}

func TestConnectionCodeGenerator_Validate(t *testing.T) {
	generator := NewConnectionCodeGenerator(nil)

	tests := []struct {
		name    string
		code    string
		wantErr bool
	}{
		{
			name:    "valid code",
			code:    "abc-def-123",
			wantErr: false,
		},
		{
			name:    "too few segments",
			code:    "abc-def",
			wantErr: true,
		},
		{
			name:    "too many segments",
			code:    "abc-def-123-xyz",
			wantErr: true,
		},
		{
			name:    "segment too short",
			code:    "ab-def-123",
			wantErr: true,
		},
		{
			name:    "segment too long",
			code:    "abcd-def-123",
			wantErr: true,
		},
		{
			name:    "invalid character (i)",
			code:    "abi-def-123", // 'i' is excluded
			wantErr: true,
		},
		{
			name:    "invalid character (l)",
			code:    "abl-def-123", // 'l' is excluded
			wantErr: true,
		},
		{
			name:    "invalid character (o)",
			code:    "abo-def-123", // 'o' is excluded
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := generator.Validate(tt.code)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConnectionCodeGenerator_GenerateUnique(t *testing.T) {
	generator := NewConnectionCodeGenerator(nil)

	existingCodes := make(map[string]bool)

	checkExists := func(code string) (bool, error) {
		return existingCodes[code], nil
	}

	// 生成第一个码
	code1, err := generator.GenerateUnique(checkExists)
	if err != nil {
		t.Fatalf("Failed to generate first unique code: %v", err)
	}
	existingCodes[code1] = true

	// 生成第二个码（应该不同）
	code2, err := generator.GenerateUnique(checkExists)
	if err != nil {
		t.Fatalf("Failed to generate second unique code: %v", err)
	}

	if code1 == code2 {
		t.Errorf("Generated duplicate codes: %s", code1)
	}

	t.Logf("Generated unique codes: %s, %s", code1, code2)
}

func TestConnectionCodeGenerator_CustomConfig(t *testing.T) {
	config := &models.ConnectionCodeGenerator{
		SegmentLength: 4,
		SegmentCount:  2,
		Separator:     "_",
		Charset:       "0123456789",
	}

	generator := NewConnectionCodeGenerator(config)

	code, err := generator.Generate()
	if err != nil {
		t.Fatalf("Failed to generate code with custom config: %v", err)
	}

	t.Logf("Generated code with custom config: %s", code)

	// 验证格式
	parts := strings.Split(code, "_")
	if len(parts) != 2 {
		t.Errorf("Expected 2 segments, got %d", len(parts))
	}

	for i, part := range parts {
		if len(part) != 4 {
			t.Errorf("Segment %d has length %d, expected 4", i, len(part))
		}

		// 验证只包含数字
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				t.Errorf("Segment %d contains non-digit character: %c", i, ch)
			}
		}
	}
}

func TestConnectionCodeGenerator_CalculateEntropy(t *testing.T) {
	generator := NewConnectionCodeGenerator(nil)

	entropy := generator.CalculateEntropy()
	entropyStr := generator.GetEntropyString()

	config := generator.GetConfig()
	t.Logf("Charset size: %d", len(config.Charset))
	t.Logf("Total length: %d", config.SegmentCount*config.SegmentLength)
	t.Logf("Entropy: %s", entropyStr)

	// 验证熵值 >= 33^9 (默认配置)
	expected := new(big.Int)
	expected.Exp(big.NewInt(33), big.NewInt(9), nil) // 33 chars, 9 total length

	if entropy.Cmp(expected) < 0 {
		t.Errorf("Entropy %s is less than expected %s", entropy.String(), expected.String())
	}
}

func TestConnectionCodeGenerator_DefaultConfig(t *testing.T) {
	generator := NewConnectionCodeGenerator(nil)
	config := generator.GetConfig()

	// 验证默认配置
	if config.SegmentLength != 3 {
		t.Errorf("Expected SegmentLength 3, got %d", config.SegmentLength)
	}

	if config.SegmentCount != 3 {
		t.Errorf("Expected SegmentCount 3, got %d", config.SegmentCount)
	}

	if config.Separator != "-" {
		t.Errorf("Expected Separator '-', got '%s'", config.Separator)
	}

	// 验证字符集排除了 i, l, o
	if strings.ContainsAny(config.Charset, "ilo") {
		t.Errorf("Charset should not contain 'i', 'l', or 'o'")
	}

	// 验证字符集包含其他字符
	if !strings.ContainsRune(config.Charset, 'a') {
		t.Error("Charset should contain 'a'")
	}
	if !strings.ContainsRune(config.Charset, '0') {
		t.Error("Charset should contain '0'")
	}
}

// 基准测试
func BenchmarkConnectionCodeGenerator_Generate(b *testing.B) {
	generator := NewConnectionCodeGenerator(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := generator.Generate()
		if err != nil {
			b.Fatalf("Failed to generate code: %v", err)
		}
	}
}

func BenchmarkConnectionCodeGenerator_Validate(b *testing.B) {
	generator := NewConnectionCodeGenerator(nil)
	code := "abc-def-123"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := generator.Validate(code)
		if err != nil {
			b.Fatalf("Failed to validate code: %v", err)
		}
	}
}
