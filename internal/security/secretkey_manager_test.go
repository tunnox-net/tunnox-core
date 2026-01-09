package security

import (
	"encoding/base64"
	"testing"
)

// 测试用主密钥（32字节）
var testMasterKey = base64.StdEncoding.EncodeToString([]byte("12345678901234567890123456789012"))

func TestNewSecretKeyManager(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := &SecretKeyConfig{MasterKey: testMasterKey}
		mgr, err := NewSecretKeyManager(config)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if mgr == nil {
			t.Fatal("expected manager to be created")
		}
	})

	t.Run("empty master key", func(t *testing.T) {
		config := &SecretKeyConfig{MasterKey: ""}
		_, err := NewSecretKeyManager(config)
		if err == nil {
			t.Fatal("expected error for empty master key")
		}
	})

	t.Run("invalid master key format", func(t *testing.T) {
		config := &SecretKeyConfig{MasterKey: "not-base64!!!"}
		_, err := NewSecretKeyManager(config)
		if err == nil {
			t.Fatal("expected error for invalid base64")
		}
	})

	t.Run("wrong key length", func(t *testing.T) {
		config := &SecretKeyConfig{MasterKey: base64.StdEncoding.EncodeToString([]byte("short"))}
		_, err := NewSecretKeyManager(config)
		if err == nil {
			t.Fatal("expected error for wrong key length")
		}
	})
}

func TestGenerateCredentials(t *testing.T) {
	config := &SecretKeyConfig{MasterKey: testMasterKey}
	mgr, err := NewSecretKeyManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	plaintext, encrypted, err := mgr.GenerateCredentials()
	if err != nil {
		t.Fatalf("failed to generate credentials: %v", err)
	}

	// 检查明文长度（64字符 = 32字节的十六进制）
	if len(plaintext) != 64 {
		t.Errorf("expected plaintext length 64, got %d", len(plaintext))
	}

	// 检查加密后的值不为空
	if encrypted == "" {
		t.Error("encrypted value should not be empty")
	}

	// 检查加密后的值是有效的 Base64
	_, err = base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		t.Errorf("encrypted value should be valid base64: %v", err)
	}
}

func TestEncryptDecrypt(t *testing.T) {
	config := &SecretKeyConfig{MasterKey: testMasterKey}
	mgr, err := NewSecretKeyManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	t.Run("encrypt and decrypt", func(t *testing.T) {
		original := "my-secret-key-12345"

		encrypted, err := mgr.Encrypt(original)
		if err != nil {
			t.Fatalf("failed to encrypt: %v", err)
		}

		decrypted, err := mgr.Decrypt(encrypted)
		if err != nil {
			t.Fatalf("failed to decrypt: %v", err)
		}

		if decrypted != original {
			t.Errorf("expected %q, got %q", original, decrypted)
		}
	})

	t.Run("different encryptions produce different ciphertexts", func(t *testing.T) {
		original := "my-secret-key"

		encrypted1, _ := mgr.Encrypt(original)
		encrypted2, _ := mgr.Encrypt(original)

		// 由于使用随机 nonce，每次加密结果应不同
		if encrypted1 == encrypted2 {
			t.Error("encryptions should produce different ciphertexts due to random nonce")
		}

		// 但解密后应该相同
		decrypted1, _ := mgr.Decrypt(encrypted1)
		decrypted2, _ := mgr.Decrypt(encrypted2)
		if decrypted1 != decrypted2 {
			t.Error("decrypted values should be identical")
		}
	})

	t.Run("invalid encrypted data", func(t *testing.T) {
		_, err := mgr.Decrypt("invalid-base64!!!")
		if err == nil {
			t.Error("expected error for invalid base64")
		}
	})

	t.Run("tampered ciphertext", func(t *testing.T) {
		encrypted, _ := mgr.Encrypt("original")

		// 解码并篡改
		data, _ := base64.StdEncoding.DecodeString(encrypted)
		data[len(data)-1] ^= 0xFF // 翻转最后一个字节
		tampered := base64.StdEncoding.EncodeToString(data)

		_, err := mgr.Decrypt(tampered)
		if err == nil {
			t.Error("expected error for tampered ciphertext")
		}
	})
}

func TestChallengeResponse(t *testing.T) {
	config := &SecretKeyConfig{MasterKey: testMasterKey}
	mgr, err := NewSecretKeyManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	t.Run("generate challenge", func(t *testing.T) {
		challenge, err := mgr.GenerateChallenge()
		if err != nil {
			t.Fatalf("failed to generate challenge: %v", err)
		}

		// 检查长度（64字符 = 32字节的十六进制）
		if len(challenge) != 64 {
			t.Errorf("expected challenge length 64, got %d", len(challenge))
		}
	})

	t.Run("different challenges", func(t *testing.T) {
		challenge1, _ := mgr.GenerateChallenge()
		challenge2, _ := mgr.GenerateChallenge()

		if challenge1 == challenge2 {
			t.Error("challenges should be unique")
		}
	})

	t.Run("verify valid response", func(t *testing.T) {
		secretKey := "my-secret-key-for-testing"
		challenge := "test-challenge-12345"

		// 加密 SecretKey
		encrypted, _ := mgr.Encrypt(secretKey)

		// 计算响应（模拟客户端）
		response := mgr.ComputeResponse(secretKey, challenge)

		// 验证（服务端）
		if !mgr.VerifyResponse(encrypted, challenge, response) {
			t.Error("expected valid response to pass verification")
		}
	})

	t.Run("verify invalid response", func(t *testing.T) {
		secretKey := "my-secret-key"
		challenge := "test-challenge"
		wrongResponse := "wrong-response"

		encrypted, _ := mgr.Encrypt(secretKey)

		if mgr.VerifyResponse(encrypted, challenge, wrongResponse) {
			t.Error("expected invalid response to fail verification")
		}
	})

	t.Run("verify with wrong secret key", func(t *testing.T) {
		secretKey := "correct-secret-key"
		wrongKey := "wrong-secret-key"
		challenge := "test-challenge"

		encrypted, _ := mgr.Encrypt(secretKey)

		// 用错误的密钥计算响应
		wrongResponse := mgr.ComputeResponse(wrongKey, challenge)

		if mgr.VerifyResponse(encrypted, challenge, wrongResponse) {
			t.Error("expected response with wrong key to fail verification")
		}
	})

	t.Run("full flow simulation", func(t *testing.T) {
		// 1. 生成凭据（服务端）
		plaintext, encrypted, _ := mgr.GenerateCredentials()

		// 2. 生成挑战（服务端）
		challenge, _ := mgr.GenerateChallenge()

		// 3. 计算响应（客户端，使用明文 SecretKey）
		response := mgr.ComputeResponse(plaintext, challenge)

		// 4. 验证响应（服务端，使用加密存储的 SecretKey）
		if !mgr.VerifyResponse(encrypted, challenge, response) {
			t.Error("full flow simulation failed")
		}
	})
}

func TestGenerateMasterKey(t *testing.T) {
	key, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("failed to generate master key: %v", err)
	}

	// 验证是有效的 Base64
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		t.Fatalf("master key should be valid base64: %v", err)
	}

	// 验证长度是 32 字节
	if len(decoded) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(decoded))
	}

	// 验证可以用于创建 manager
	config := &SecretKeyConfig{MasterKey: key}
	_, err = NewSecretKeyManager(config)
	if err != nil {
		t.Errorf("generated key should be usable: %v", err)
	}
}
