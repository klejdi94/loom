// Package core provides fundamental prompt types and interfaces for the loom framework.
package core

import "errors"

// Sentinel errors for prompt operations.
var (
	ErrPromptNotFound   = errors.New("prompt not found")
	ErrInvalidVersion   = errors.New("invalid version format")
	ErrValidationFailed = errors.New("validation failed")
	ErrRenderFailed     = errors.New("template render failed")
)

// ValidationError carries field-level validation context.
type ValidationError struct {
	Field   string
	Value   interface{}
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}
