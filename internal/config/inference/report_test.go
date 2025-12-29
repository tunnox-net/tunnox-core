package inference

import (
	"strings"
	"testing"
)

func TestNewReporter(t *testing.T) {
	result := NewInferenceResult()
	reporter := NewReporter(result)

	if reporter == nil {
		t.Fatal("NewReporter returned nil")
	}
	if reporter.result != result {
		t.Error("Reporter result does not match input")
	}
}

func TestReporter_FormatReport_Empty(t *testing.T) {
	result := NewInferenceResult()
	reporter := NewReporter(result)

	report := reporter.FormatReport()

	if !strings.Contains(report, "No changes applied") {
		t.Error("Expected 'No changes applied' in empty report")
	}
}

func TestReporter_FormatReport_WithApplied(t *testing.T) {
	result := NewInferenceResult()
	result.AddApplied("storage.type", "hybrid", "remote storage requires hybrid")

	reporter := NewReporter(result)
	report := reporter.FormatReport()

	if !strings.Contains(report, "Auto-inferred settings") {
		t.Error("Expected 'Auto-inferred settings' in report")
	}
	if !strings.Contains(report, "storage.type") {
		t.Error("Expected field name in report")
	}
	if !strings.Contains(report, "hybrid") {
		t.Error("Expected value in report")
	}
	if !strings.Contains(report, "remote storage requires hybrid") {
		t.Error("Expected reason in report")
	}
}

func TestReporter_FormatReport_WithSkipped(t *testing.T) {
	result := NewInferenceResult()
	result.AddSkipped("storage.persistence.enabled", true, "user override")

	reporter := NewReporter(result)
	report := reporter.FormatReport()

	if !strings.Contains(report, "Skipped inferences") {
		t.Error("Expected 'Skipped inferences' in report")
	}
	if !strings.Contains(report, "storage.persistence.enabled") {
		t.Error("Expected field name in report")
	}
	if !strings.Contains(report, "user override") {
		t.Error("Expected reason in report")
	}
}

func TestReporter_FormatReport_WithWarnings(t *testing.T) {
	result := NewInferenceResult()
	result.AddWarning("platform.enabled=true but storage.remote.enabled=false")

	reporter := NewReporter(result)
	report := reporter.FormatReport()

	if !strings.Contains(report, "Warnings") {
		t.Error("Expected 'Warnings' in report")
	}
	if !strings.Contains(report, "platform.enabled=true but storage.remote.enabled=false") {
		t.Error("Expected warning message in report")
	}
}

func TestReporter_FormatReport_Complete(t *testing.T) {
	result := NewInferenceResult()
	result.AddApplied("storage.type", "hybrid", "remote storage requires hybrid")
	result.AddApplied("storage.persistence.enabled", true, "remote storage requires caching")
	result.AddSkipped("storage.remote.enabled", true, "already set by user")
	result.AddWarning("some warning message")

	reporter := NewReporter(result)
	report := reporter.FormatReport()

	// Check structure
	if !strings.Contains(report, "=== Tunnox Configuration Inference Report ===") {
		t.Error("Expected report header")
	}
	if !strings.Contains(report, "Auto-inferred settings") {
		t.Error("Expected applied section")
	}
	if !strings.Contains(report, "Skipped inferences") {
		t.Error("Expected skipped section")
	}
	if !strings.Contains(report, "Warnings") {
		t.Error("Expected warnings section")
	}
}

func TestReporter_FormatReport_NilResult(t *testing.T) {
	reporter := NewReporter(nil)
	report := reporter.FormatReport()

	if report != "" {
		t.Error("Expected empty report for nil result")
	}
}

func TestPrintReport_NilResult(t *testing.T) {
	// Should not panic
	PrintReport(nil)
}

func TestPrintReport_EmptyResult(t *testing.T) {
	// Should not panic
	result := NewInferenceResult()
	PrintReport(result)
}
