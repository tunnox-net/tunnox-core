package validation

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	coreErrors "tunnox-core/internal/core/errors"
)

// Validator 配置验证器接口
type Validator interface {
	// Validate 验证配置，返回验证错误列表
	Validate() []error
}

// ValidationResult 验证结果
type ValidationResult struct {
	Errors []error
}

// IsValid 检查验证是否通过
func (r *ValidationResult) IsValid() bool {
	return len(r.Errors) == 0
}

// Error 实现 error 接口
func (r *ValidationResult) Error() string {
	if r.IsValid() {
		return ""
	}
	var msgs []string
	for _, err := range r.Errors {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// AddError 添加验证错误
func (r *ValidationResult) AddError(err error) {
	if err != nil {
		r.Errors = append(r.Errors, err)
	}
}

// AddErrorf 添加格式化的验证错误
func (r *ValidationResult) AddErrorf(errorType coreErrors.ErrorType, format string, args ...interface{}) {
	r.AddError(coreErrors.Newf(errorType, format, args...))
}

// ValidatePort 验证端口号
func ValidatePort(port int, fieldName string) error {
	if port < 1 || port > 65535 {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: port %d out of range [1, 65535]", fieldName, port)
	}
	return nil
}

// ValidatePortOrZero 验证端口号（允许0，表示使用默认值）
func ValidatePortOrZero(port int, fieldName string) error {
	if port < 0 || port > 65535 {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: port %d out of range [0, 65535]", fieldName, port)
	}
	return nil
}

// ValidateTimeout 验证超时时间（秒）
func ValidateTimeout(timeout int, fieldName string) error {
	if timeout < 0 {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: timeout %d cannot be negative", fieldName, timeout)
	}
	if timeout == 0 {
		// 0 表示使用默认值，允许
		return nil
	}
	// 检查是否过大（超过1年）
	maxTimeout := int((365 * 24 * time.Hour).Seconds())
	if timeout > maxTimeout {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: timeout %d is too large (max: %d seconds)", fieldName, timeout, maxTimeout)
	}
	return nil
}

// ValidateDuration 验证持续时间（秒）
func ValidateDuration(duration int, fieldName string) error {
	return ValidateTimeout(duration, fieldName)
}

// ValidateNonEmptyString 验证非空字符串
func ValidateNonEmptyString(value, fieldName string) error {
	if strings.TrimSpace(value) == "" {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: cannot be empty", fieldName)
	}
	return nil
}

// ValidateStringInList 验证字符串是否在允许的列表中
func ValidateStringInList(value, fieldName string, allowedValues []string) error {
	if value == "" {
		return nil // 空值由其他验证处理
	}
	for _, allowed := range allowedValues {
		if value == allowed {
			return nil
		}
	}
	return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: invalid value %q, must be one of: %v", fieldName, value, allowedValues)
}

// ValidateHost 验证主机地址
func ValidateHost(host, fieldName string) error {
	if host == "" {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: host cannot be empty", fieldName)
	}
	// 验证是否为有效的 IP 地址或主机名
	if host != "0.0.0.0" && host != "::" && host != "localhost" {
		if ip := net.ParseIP(host); ip == nil {
			// 不是 IP 地址，检查是否为有效的主机名
			if _, err := net.LookupHost(host); err != nil {
				// 注意：这里不返回错误，因为可能是运行时解析
				// 只检查格式
				if strings.Contains(host, " ") {
					return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: host %q contains invalid characters", fieldName, host)
				}
			}
		}
	}
	return nil
}

// ValidateAddress 验证地址格式（host:port）
func ValidateAddress(addr, fieldName string) error {
	if addr == "" {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: address cannot be empty", fieldName)
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: invalid address format %q: %v", fieldName, addr, err)
	}
	if err := ValidateHost(host, fieldName+".host"); err != nil {
		return err
	}
	// 尝试解析端口号（先尝试作为服务名，再尝试作为数字）
	port, err := net.LookupPort("tcp", portStr)
	if err != nil {
		// 如果 LookupPort 失败，尝试直接解析为数字
		portNum, parseErr := strconv.Atoi(portStr)
		if parseErr != nil {
			return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: invalid port %q in address %q", fieldName, portStr, addr)
		}
		port = portNum
	}
	if err := ValidatePort(port, fieldName+".port"); err != nil {
		return err
	}
	return nil
}

// ValidatePositiveInt 验证正整数
func ValidatePositiveInt(value int, fieldName string) error {
	if value <= 0 {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: must be positive, got %d", fieldName, value)
	}
	return nil
}

// ValidateNonNegativeInt 验证非负整数
func ValidateNonNegativeInt(value int, fieldName string) error {
	if value < 0 {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: cannot be negative, got %d", fieldName, value)
	}
	return nil
}

// ValidateIntRange 验证整数范围
func ValidateIntRange(value, min, max int, fieldName string) error {
	if value < min || value > max {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: value %d out of range [%d, %d]", fieldName, value, min, max)
	}
	return nil
}

// ValidateInt64Range 验证 int64 范围
func ValidateInt64Range(value, min, max int64, fieldName string) error {
	if value < min || value > max {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: value %d out of range [%d, %d]", fieldName, value, min, max)
	}
	return nil
}

// ValidateURL 验证 URL 格式
func ValidateURL(url, fieldName string) error {
	if url == "" {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: URL cannot be empty", fieldName)
	}
	// 基本格式检查
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: URL %q must start with http:// or https://", fieldName, url)
	}
	return nil
}

// ValidateCompressionLevel 验证压缩级别
func ValidateCompressionLevel(level int, fieldName string) error {
	return ValidateIntRange(level, 1, 9, fieldName)
}

// ValidateBandwidthLimit 验证带宽限制（字节/秒）
func ValidateBandwidthLimit(limit int64, fieldName string) error {
	if limit < 0 {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: bandwidth limit cannot be negative", fieldName)
	}
	// 检查是否过大（1TB/s）
	maxBandwidth := int64(1024 * 1024 * 1024 * 1024)
	if limit > maxBandwidth {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: bandwidth limit %d is too large (max: %d bytes/s)", fieldName, limit, maxBandwidth)
	}
	return nil
}

// ValidateMaxConnections 验证最大连接数
func ValidateMaxConnections(maxConn int, fieldName string) error {
	if maxConn < 0 {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: max connections cannot be negative", fieldName)
	}
	// 检查是否过大（1000万）
	maxConnections := 10 * 1000 * 1000
	if maxConn > maxConnections {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: max connections %d is too large (max: %d)", fieldName, maxConn, maxConnections)
	}
	return nil
}

// ValidatePortList 验证端口列表
func ValidatePortList(ports []int, fieldName string) error {
	for i, port := range ports {
		if err := ValidatePort(port, fmt.Sprintf("%s[%d]", fieldName, i)); err != nil {
			return err
		}
	}
	return nil
}

// ValidateRequired 验证必填字段
func ValidateRequired(value interface{}, fieldName string) error {
	if value == nil {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: is required", fieldName)
	}
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: is required", fieldName)
		}
	case int:
		if v == 0 {
			return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: is required", fieldName)
		}
	case int64:
		if v == 0 {
			return coreErrors.Newf(coreErrors.ErrorTypePermanent, "%s: is required", fieldName)
		}
	}
	return nil
}

