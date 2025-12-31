package conncode

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	"tunnox-core/internal/cloud/models"
	coreerrors "tunnox-core/internal/core/errors"
)

// Generator 连接码生成器
// 职责：生成好记的连接码（如 abc-def-123）
type Generator struct {
	config *models.ConnectionCodeGenerator
}

// NewGenerator 创建连接码生成器
func NewGenerator(config *models.ConnectionCodeGenerator) *Generator {
	if config == nil {
		config = models.DefaultConnectionCodeGenerator()
	}
	return &Generator{
		config: config,
	}
}

// Generate 生成一个连接码
// 返回格式：abc-def-123 (3段 × 3字符，用 - 分隔)
func (g *Generator) Generate() (string, error) {
	segments := make([]string, g.config.SegmentCount)

	for i := 0; i < g.config.SegmentCount; i++ {
		segment, err := g.generateSegment()
		if err != nil {
			return "", coreerrors.Wrapf(err, coreerrors.CodeInternal, "failed to generate segment %d", i)
		}
		segments[i] = segment
	}

	return strings.Join(segments, g.config.Separator), nil
}

// generateSegment 生成一个段（如 "abc"）
func (g *Generator) generateSegment() (string, error) {
	charsetLen := int64(len(g.config.Charset))
	if charsetLen == 0 {
		return "", coreerrors.New(coreerrors.CodeConfigError, "charset is empty")
	}

	segment := make([]byte, g.config.SegmentLength)
	for i := 0; i < g.config.SegmentLength; i++ {
		// 使用 crypto/rand 生成安全的随机数
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(charsetLen))
		if err != nil {
			return "", coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to generate random number")
		}
		segment[i] = g.config.Charset[randomIndex.Int64()]
	}

	return string(segment), nil
}

// GenerateUnique 生成唯一的连接码（带唯一性检查）
// checkExists 函数用于检查连接码是否已存在
func (g *Generator) GenerateUnique(checkExists func(string) (bool, error)) (string, error) {
	maxAttempts := 100 // 防止无限循环

	for attempt := 0; attempt < maxAttempts; attempt++ {
		code, err := g.Generate()
		if err != nil {
			return "", coreerrors.Wrapf(err, coreerrors.CodeInternal, "failed to generate code (attempt %d)", attempt+1)
		}

		exists, err := checkExists(code)
		if err != nil {
			return "", coreerrors.Wrapf(err, coreerrors.CodeStorageError, "failed to check code existence (attempt %d)", attempt+1)
		}

		if !exists {
			return code, nil
		}

		// 已存在，重试
	}

	return "", coreerrors.Newf(coreerrors.CodeResourceExhausted, "failed to generate unique code after %d attempts", maxAttempts)
}

// Validate 验证连接码格式是否正确
func (g *Generator) Validate(code string) error {
	// 1. 检查分隔符数量
	parts := strings.Split(code, g.config.Separator)
	if len(parts) != g.config.SegmentCount {
		return coreerrors.Newf(coreerrors.CodeValidationError, "invalid code format: expected %d segments, got %d",
			g.config.SegmentCount, len(parts))
	}

	// 2. 检查每段长度
	for i, part := range parts {
		if len(part) != g.config.SegmentLength {
			return coreerrors.Newf(coreerrors.CodeValidationError, "invalid segment %d: expected length %d, got %d",
				i, g.config.SegmentLength, len(part))
		}

		// 3. 检查字符是否在字符集中
		for _, ch := range part {
			if !strings.ContainsRune(g.config.Charset, ch) {
				return coreerrors.Newf(coreerrors.CodeValidationError, "invalid character '%c' in segment %d", ch, i)
			}
		}
	}

	return nil
}

// CalculateEntropy 计算连接码的熵值（安全性指标）
// 返回可能的组合数（如 33^9 ≈ 4.6 × 10^13）
func (g *Generator) CalculateEntropy() *big.Int {
	charsetSize := big.NewInt(int64(len(g.config.Charset)))
	totalLength := g.config.SegmentCount * g.config.SegmentLength

	// 熵值 = charset_size ^ total_length
	entropy := new(big.Int)
	entropy.Exp(charsetSize, big.NewInt(int64(totalLength)), nil)

	return entropy
}

// GetConfig 获取配置（用于测试和调试）
func (g *Generator) GetConfig() *models.ConnectionCodeGenerator {
	return g.config
}

// GetEntropyString 获取熵值的可读字符串
func (g *Generator) GetEntropyString() string {
	entropy := g.CalculateEntropy()

	// 格式化为科学记数法（如果太大）
	entropyFloat := new(big.Float).SetInt(entropy)

	// 计算 log10
	log10 := new(big.Float)
	log10.SetPrec(100)

	// 简化：如果大于 10^12，使用科学记数法
	trillion := big.NewInt(1000000000000) // 10^12
	if entropy.Cmp(trillion) > 0 {
		// 计算指数
		exponent := 0
		temp := new(big.Int).Set(entropy)
		for temp.Cmp(big.NewInt(10)) >= 0 {
			temp.Div(temp, big.NewInt(10))
			exponent++
		}

		// 计算尾数
		mantissa := new(big.Float).SetInt(entropy)
		divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil))
		mantissa.Quo(mantissa, divisor)

		mantissaStr, _ := mantissa.Float64()
		return fmt.Sprintf("%.2e (≈ %.1f × 10^%d)", entropyFloat, mantissaStr, exponent)
	}

	return entropy.String()
}
