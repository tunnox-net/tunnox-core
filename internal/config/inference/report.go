package inference

import (
	"fmt"
	"strings"

	corelog "tunnox-core/internal/core/log"
)

// Reporter formats and outputs inference results
type Reporter struct {
	result *InferenceResult
}

// NewReporter creates a new Reporter for the given result
func NewReporter(result *InferenceResult) *Reporter {
	return &Reporter{result: result}
}

// LogReport outputs the inference report to the log
func (r *Reporter) LogReport() {
	if r.result == nil {
		return
	}

	// Only log if there are changes, skips, or warnings
	if !r.result.HasChanges() && len(r.result.Skipped) == 0 && !r.result.HasWarnings() {
		corelog.Debug("Configuration inference: no changes applied")
		return
	}

	corelog.Info("=== Tunnox Configuration Inference Report ===")

	// Log applied inferences
	if len(r.result.Applied) > 0 {
		corelog.Info("")
		corelog.Info("Auto-inferred settings:")
		for _, action := range r.result.Applied {
			corelog.Infof("  + %s = %v", action.Field, action.Value)
			corelog.Infof("    Reason: %s", action.Reason)
		}
	}

	// Log skipped inferences
	if len(r.result.Skipped) > 0 {
		corelog.Info("")
		corelog.Info("Skipped inferences (user override):")
		for _, action := range r.result.Skipped {
			corelog.Infof("  - %s", action.Field)
			corelog.Infof("    Would have been: %v", action.Value)
			corelog.Infof("    Reason: %s", action.Reason)
		}
	}

	// Log warnings
	if len(r.result.Warnings) > 0 {
		corelog.Info("")
		corelog.Warn("Warnings:")
		for _, warning := range r.result.Warnings {
			corelog.Warnf("  ! %s", warning)
		}
	}

	corelog.Info("")
}

// FormatReport returns a formatted string representation of the report
func (r *Reporter) FormatReport() string {
	if r.result == nil {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("=== Tunnox Configuration Inference Report ===\n")

	// Format applied inferences
	if len(r.result.Applied) > 0 {
		sb.WriteString("\nAuto-inferred settings:\n")
		for _, action := range r.result.Applied {
			sb.WriteString(fmt.Sprintf("  + %s = %v\n", action.Field, action.Value))
			sb.WriteString(fmt.Sprintf("    Reason: %s\n", action.Reason))
		}
	}

	// Format skipped inferences
	if len(r.result.Skipped) > 0 {
		sb.WriteString("\nSkipped inferences (user override):\n")
		for _, action := range r.result.Skipped {
			sb.WriteString(fmt.Sprintf("  - %s\n", action.Field))
			sb.WriteString(fmt.Sprintf("    Would have been: %v\n", action.Value))
			sb.WriteString(fmt.Sprintf("    Reason: %s\n", action.Reason))
		}
	}

	// Format warnings
	if len(r.result.Warnings) > 0 {
		sb.WriteString("\nWarnings:\n")
		for _, warning := range r.result.Warnings {
			sb.WriteString(fmt.Sprintf("  ! %s\n", warning))
		}
	}

	if len(r.result.Applied) == 0 && len(r.result.Skipped) == 0 && len(r.result.Warnings) == 0 {
		sb.WriteString("\nNo changes applied.\n")
	}

	return sb.String()
}

// PrintReport outputs the inference report to the log using a formatted string
func PrintReport(result *InferenceResult) {
	reporter := NewReporter(result)
	reporter.LogReport()
}
