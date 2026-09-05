package core

import (
	"errors"
	"fmt"
)

// ErrorCode represents a domain error code mapped to SPEC §8.2.
type ErrorCode string

const (
	ErrNotFound                     ErrorCode = "not_found"
	ErrAmbiguousTarget              ErrorCode = "ambiguous_target"
	ErrConflictDetected             ErrorCode = "conflict_detected"
	ErrInvalidFrontmatter           ErrorCode = "invalid_frontmatter"
	ErrArtifactTooLarge             ErrorCode = "artifact_too_large"
	ErrBinaryArtifactUnsupported    ErrorCode = "binary_artifact_unsupported"
	ErrTranscriptUnavailable        ErrorCode = "transcript_unavailable"
	ErrOverTokenTarget              ErrorCode = "over_token_target"
	ErrConcurrentEdit               ErrorCode = "concurrent_edit"
	ErrHarnessCapabilityUnavailable ErrorCode = "harness_capability_unavailable"
	ErrInternal                     ErrorCode = "internal_error"
)

// DomainError encapsulates typed errors for unified rendering across CLI, MCP, and TUI.
type DomainError struct {
	Code    ErrorCode
	Message string
	Err     error
}

func (e *DomainError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *DomainError) Unwrap() error {
	return e.Err
}

// NewError creates a new DomainError without an underlying error.
func NewError(code ErrorCode, message string) *DomainError {
	return &DomainError{
		Code:    code,
		Message: message,
	}
}

// WrapError creates a new DomainError wrapping an underlying error.
func WrapError(code ErrorCode, message string, err error) *DomainError {
	return &DomainError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// ErrInstallSkipped means the adapter deliberately wrote nothing.
var ErrInstallSkipped = errors.New("install skipped")
