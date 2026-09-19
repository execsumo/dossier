package mcp

import (
	"dossier/internal/core"
)

// MCPErrorCode maps domain errors to the specific string error codes required by the SPEC.
type MCPErrorCode string

const (
	ErrCodeNotFound                  MCPErrorCode = "not_found"
	ErrCodeAmbiguousTarget           MCPErrorCode = "ambiguous_target"
	ErrCodeConflictDetected          MCPErrorCode = "conflict_detected"
	ErrCodeInvalidFrontmatter        MCPErrorCode = "invalid_frontmatter"
	ErrCodeArtifactTooLarge          MCPErrorCode = "artifact_too_large"
	ErrCodeBinaryArtifactUnsupported MCPErrorCode = "binary_artifact_unsupported"
	ErrCodeTranscriptUnavailable     MCPErrorCode = "transcript_unavailable"
	ErrCodeOverTokenTarget           MCPErrorCode = "over_token_target"
	ErrCodeConcurrentEdit            MCPErrorCode = "concurrent_edit"
	ErrCodeHarnessCapUnavailable     MCPErrorCode = "harness_capability_unavailable"
	ErrCodeInternal                  MCPErrorCode = "internal_error"
	ErrCodeSyncAuthFailed            MCPErrorCode = "sync_auth_failed"
)

// MapError converts a core.DomainError to its corresponding MCPErrorCode and message.
func MapError(err error) (MCPErrorCode, string) {
	if err == nil {
		return "", ""
	}

	coreErr, ok := err.(*core.DomainError)
	if !ok {
		return ErrCodeInternal, err.Error()
	}

	switch coreErr.Code {
	case core.ErrNotFound:
		return ErrCodeNotFound, coreErr.Message
	case core.ErrAmbiguousTarget:
		return ErrCodeAmbiguousTarget, coreErr.Message
	case core.ErrConflictDetected:
		return ErrCodeConflictDetected, coreErr.Message
	case core.ErrInvalidFrontmatter:
		return ErrCodeInvalidFrontmatter, coreErr.Message
	case core.ErrArtifactTooLarge:
		return ErrCodeArtifactTooLarge, coreErr.Message
	case core.ErrBinaryArtifactUnsupported:
		return ErrCodeBinaryArtifactUnsupported, coreErr.Message
	case core.ErrTranscriptUnavailable:
		return ErrCodeTranscriptUnavailable, coreErr.Message
	case core.ErrOverTokenTarget:
		return ErrCodeOverTokenTarget, coreErr.Message
	case core.ErrConcurrentEdit:
		return ErrCodeConcurrentEdit, coreErr.Message
	case core.ErrHarnessCapabilityUnavailable:
		return ErrCodeHarnessCapUnavailable, coreErr.Message
	case core.ErrSyncAuthFailed:
		return ErrCodeSyncAuthFailed, coreErr.Message
	default:
		return ErrCodeInternal, coreErr.Message
	}
}
