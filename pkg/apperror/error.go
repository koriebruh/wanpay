package apperror

import (
	"fmt"
	"net/http"
)

type AppError struct {
	Code    int          `json:"-"`
	Message string       `json:"message"`
	Details []FieldError `json:"details,omitempty"`
}

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *AppError) Error() string { return e.Message }

// WithDetails attaches field-level validation errors to the response.
func (e *AppError) WithDetails(details ...FieldError) *AppError {
	e.Details = append(e.Details, details...)
	return e
}

func BadRequest(format string, args ...any) *AppError {
	return &AppError{Code: http.StatusBadRequest, Message: fmt.Sprintf(format, args...)}
}

func Unauthorized(format string, args ...any) *AppError {
	return &AppError{Code: http.StatusUnauthorized, Message: fmt.Sprintf(format, args...)}
}

func Forbidden(format string, args ...any) *AppError {
	return &AppError{Code: http.StatusForbidden, Message: fmt.Sprintf(format, args...)}
}

func NotFound(format string, args ...any) *AppError {
	return &AppError{Code: http.StatusNotFound, Message: fmt.Sprintf(format, args...)}
}

func Conflict(format string, args ...any) *AppError {
	return &AppError{Code: http.StatusConflict, Message: fmt.Sprintf(format, args...)}
}

func UnprocessableEntity(format string, args ...any) *AppError {
	return &AppError{Code: http.StatusUnprocessableEntity, Message: fmt.Sprintf(format, args...)}
}

func Internal(format string, args ...any) *AppError {
	return &AppError{Code: http.StatusInternalServerError, Message: fmt.Sprintf(format, args...)}
}

func Is(err error) (*AppError, bool) {
	ae, ok := err.(*AppError)
	return ae, ok
}
