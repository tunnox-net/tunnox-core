package log

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

// TestNopLogger 测试静默日志
func TestNopLogger(t *testing.T) {
	logger := NewNopLogger()

	// 所有方法都不应该 panic
	logger.Debug("test")
	logger.Info("test")
	logger.Warn("test")
	logger.Error("test")
	logger.Debugf("test %s", "arg")
	logger.Infof("test %s", "arg")
	logger.Warnf("test %s", "arg")
	logger.Errorf("test %s", "arg")

	// WithField 应该返回自身
	l := logger.WithField("key", "value")
	if _, ok := l.(NopLogger); !ok {
		t.Error("WithField should return NopLogger")
	}

	// WithFields 应该返回自身
	l = logger.WithFields(map[string]interface{}{"key": "value"})
	if _, ok := l.(NopLogger); !ok {
		t.Error("WithFields should return NopLogger")
	}

	// WithError 应该返回自身
	l = logger.WithError(nil)
	if _, ok := l.(NopLogger); !ok {
		t.Error("WithError should return NopLogger")
	}

	// WithContext 应该返回自身
	l = logger.WithContext(context.Background())
	if _, ok := l.(NopLogger); !ok {
		t.Error("WithContext should return NopLogger")
	}
}

// mockTestingT 模拟 testing.T
type mockTestingT struct {
	logs []string
}

func (m *mockTestingT) Log(args ...interface{}) {
	m.logs = append(m.logs, args[0].(string))
}

func (m *mockTestingT) Logf(format string, args ...interface{}) {
	m.logs = append(m.logs, format)
}

// TestTestLogger 测试测试日志
func TestTestLogger(t *testing.T) {
	mock := &mockTestingT{}
	logger := NewTestLogger(mock)

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	if len(mock.logs) != 4 {
		t.Errorf("Expected 4 logs, got %d", len(mock.logs))
	}

	// 测试格式化方法
	mock.logs = nil
	logger.Debugf("debug %s", "formatted")
	logger.Infof("info %s", "formatted")
	logger.Warnf("warn %s", "formatted")
	logger.Errorf("error %s", "formatted")

	if len(mock.logs) != 4 {
		t.Errorf("Expected 4 logs, got %d", len(mock.logs))
	}
}

// TestLogrusLogger 测试 logrus 日志
func TestLogrusLogger(t *testing.T) {
	// 创建一个带缓冲区的 logrus logger
	var buf bytes.Buffer
	l := logrus.New()
	l.SetOutput(&buf)
	l.SetLevel(logrus.DebugLevel)
	l.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp: true,
	})

	logger := NewLogrusLogger(l)

	// 测试基础方法
	logger.Debug("debug message")
	if !strings.Contains(buf.String(), "debug message") {
		t.Error("Debug message not found in output")
	}

	buf.Reset()
	logger.Info("info message")
	if !strings.Contains(buf.String(), "info message") {
		t.Error("Info message not found in output")
	}

	buf.Reset()
	logger.Warn("warn message")
	if !strings.Contains(buf.String(), "warn message") {
		t.Error("Warn message not found in output")
	}

	buf.Reset()
	logger.Error("error message")
	if !strings.Contains(buf.String(), "error message") {
		t.Error("Error message not found in output")
	}

	// 测试 WithField
	buf.Reset()
	logger.WithField("key", "value").Info("with field")
	if !strings.Contains(buf.String(), "key=value") {
		t.Error("Field not found in output")
	}

	// 测试 WithFields
	buf.Reset()
	logger.WithFields(map[string]interface{}{"k1": "v1", "k2": "v2"}).Info("with fields")
	output := buf.String()
	if !strings.Contains(output, "k1=v1") || !strings.Contains(output, "k2=v2") {
		t.Error("Fields not found in output")
	}
}

// TestDefaultLogger 测试默认日志
func TestDefaultLogger(t *testing.T) {
	// 获取默认 logger
	logger := Default()
	if logger == nil {
		t.Fatal("Default logger should not be nil")
	}

	// 设置新的默认 logger
	nopLogger := NewNopLogger()
	SetDefault(nopLogger)

	// 验证设置成功
	if Default() != nopLogger {
		t.Error("SetDefault did not work")
	}

	// 恢复原始 logger
	SetDefault(logger)
}

// TestGlobalFunctions 测试全局函数
func TestGlobalFunctions(t *testing.T) {
	// 设置静默日志以避免输出
	SetDefault(NewNopLogger())

	// 所有全局函数都不应该 panic
	Debug("test")
	Info("test")
	Warn("test")
	Error("test")
	Debugf("test %s", "arg")
	Infof("test %s", "arg")
	Warnf("test %s", "arg")
	Errorf("test %s", "arg")

	// WithField
	l := WithField("key", "value")
	if l == nil {
		t.Error("WithField should not return nil")
	}

	// WithFields
	l = WithFields(map[string]interface{}{"key": "value"})
	if l == nil {
		t.Error("WithFields should not return nil")
	}

	// WithError
	l = WithError(nil)
	if l == nil {
		t.Error("WithError should not return nil")
	}
}
