package utils

import (
	"strings"
	"testing"
)

func TestGenerateRandomBytes(t *testing.T) {
	length := 16
	bytes, err := GenerateRandomBytes(length)
	if err != nil {
		t.Fatalf("GenerateRandomBytes failed: %v", err)
	}

	if len(bytes) != length {
		t.Errorf("Expected length %d, got %d", length, len(bytes))
	}

	// 检查是否所有字节都不相同（随机性测试）
	allSame := true
	firstByte := bytes[0]
	for _, b := range bytes {
		if b != firstByte {
			allSame = false
			break
		}
	}
	if allSame {
		t.Error("All bytes are the same, randomness might be compromised")
	}
}

func TestGenerateRandomString(t *testing.T) {
	length := 10
	str, err := GenerateRandomString(length)
	if err != nil {
		t.Fatalf("GenerateRandomString failed: %v", err)
	}

	if len(str) != length {
		t.Errorf("Expected length %d, got %d", length, len(str))
	}

	// 检查字符是否都在字符集中
	for _, char := range str {
		if !strings.ContainsRune(Charset, char) {
			t.Errorf("Character '%c' not in charset", char)
		}
	}
}

func TestGenerateRandomStringWithCharset(t *testing.T) {
	length := 8
	customCharset := "ABC123"
	str, err := GenerateRandomStringWithCharset(length, customCharset)
	if err != nil {
		t.Fatalf("GenerateRandomStringWithCharset failed: %v", err)
	}

	if len(str) != length {
		t.Errorf("Expected length %d, got %d", length, len(str))
	}

	// 检查字符是否都在自定义字符集中
	for _, char := range str {
		if !strings.ContainsRune(customCharset, char) {
			t.Errorf("Character '%c' not in custom charset", char)
		}
	}
}

func TestGenerateRandomDigits(t *testing.T) {
	length := 6
	str, err := GenerateRandomDigits(length)
	if err != nil {
		t.Fatalf("GenerateRandomDigits failed: %v", err)
	}

	if len(str) != length {
		t.Errorf("Expected length %d, got %d", length, len(str))
	}

	// 检查是否都是数字
	for _, char := range str {
		if char < '0' || char > '9' {
			t.Errorf("Character '%c' is not a digit", char)
		}
	}
}

func TestGenerateRandomInt64(t *testing.T) {
	min := int64(100)
	max := int64(200)

	for i := 0; i < 100; i++ {
		val, err := GenerateRandomInt64(min, max)
		if err != nil {
			t.Fatalf("GenerateRandomInt64 failed: %v", err)
		}

		if val < min || val > max {
			t.Errorf("Value %d is outside range [%d, %d]", val, min, max)
		}
	}
}

func TestGenerateRandomInt64InvalidRange(t *testing.T) {
	_, err := GenerateRandomInt64(200, 100)
	if err == nil {
		t.Error("Expected error for invalid range")
	}
}

func TestGenerateRandomInt(t *testing.T) {
	min := 10
	max := 50

	for i := 0; i < 100; i++ {
		val, err := GenerateRandomInt(min, max)
		if err != nil {
			t.Fatalf("GenerateRandomInt failed: %v", err)
		}

		if val < min || val > max {
			t.Errorf("Value %d is outside range [%d, %d]", val, min, max)
		}
	}
}

func TestGenerateRandomFloat64(t *testing.T) {
	for i := 0; i < 100; i++ {
		val, err := GenerateRandomFloat64()
		if err != nil {
			t.Fatalf("GenerateRandomFloat64 failed: %v", err)
		}

		if val < 0 || val > 1 {
			t.Errorf("Value %f is outside range [0, 1]", val)
		}
	}
}

func TestGenerateRandomFloat64Range(t *testing.T) {
	min := 1.5
	max := 3.5

	for i := 0; i < 100; i++ {
		val, err := GenerateRandomFloat64Range(min, max)
		if err != nil {
			t.Fatalf("GenerateRandomFloat64Range failed: %v", err)
		}

		if val < min || val > max {
			t.Errorf("Value %f is outside range [%f, %f]", val, min, max)
		}
	}
}

func TestGenerateUUID(t *testing.T) {
	uuid, err := GenerateUUID()
	if err != nil {
		t.Fatalf("GenerateUUID failed: %v", err)
	}

	// UUID格式: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	if len(uuid) != 36 {
		t.Errorf("Expected UUID length 36, got %d", len(uuid))
	}

	// 检查格式
	parts := strings.Split(uuid, "-")
	if len(parts) != 5 {
		t.Errorf("Expected 5 parts in UUID, got %d", len(parts))
	}

	expectedLengths := []int{8, 4, 4, 4, 12}
	for i, part := range parts {
		if len(part) != expectedLengths[i] {
			t.Errorf("Part %d expected length %d, got %d", i, expectedLengths[i], len(part))
		}
	}

	// 检查是否都是十六进制字符
	hexChars := "0123456789abcdef"
	for _, char := range strings.ToLower(uuid) {
		if char != '-' && !strings.ContainsRune(hexChars, char) {
			t.Errorf("Character '%c' is not valid in UUID", char)
		}
	}
}

func BenchmarkGenerateRandomString(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := GenerateRandomString(16)
		if err != nil {
			b.Fatalf("GenerateRandomString failed: %v", err)
		}
	}
}

func BenchmarkGenerateRandomInt64(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := GenerateRandomInt64(1000, 9999)
		if err != nil {
			b.Fatalf("GenerateRandomInt64 failed: %v", err)
		}
	}
}

func BenchmarkGenerateUUID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := GenerateUUID()
		if err != nil {
			b.Fatalf("GenerateUUID failed: %v", err)
		}
	}
}
