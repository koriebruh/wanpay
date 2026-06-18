package apperror

import (
	"fmt"
	"net/http"
)

type AppError struct {
	httpCode int
	Message  string       `json:"message"`
	Details  []FieldError `json:"details,omitempty"`
}

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *AppError) Error() string     { return e.Message }
func (e *AppError) HTTPCode() int     { return e.httpCode }

func (e *AppError) WithDetails(details ...FieldError) *AppError {
	e.Details = append(e.Details, details...)
	return e
}

func BadRequest(format string, args ...any) *AppError {
	return &AppError{httpCode: http.StatusBadRequest, Message: fmt.Sprintf(format, args...)}
}

func Unauthorized(format string, args ...any) *AppError {
	return &AppError{httpCode: http.StatusUnauthorized, Message: fmt.Sprintf(format, args...)}
}

func Forbidden(format string, args ...any) *AppError {
	return &AppError{httpCode: http.StatusForbidden, Message: fmt.Sprintf(format, args...)}
}

func NotFound(format string, args ...any) *AppError {
	return &AppError{httpCode: http.StatusNotFound, Message: fmt.Sprintf(format, args...)}
}

func Conflict(format string, args ...any) *AppError {
	return &AppError{httpCode: http.StatusConflict, Message: fmt.Sprintf(format, args...)}
}

func UnprocessableEntity(format string, args ...any) *AppError {
	return &AppError{httpCode: http.StatusUnprocessableEntity, Message: fmt.Sprintf(format, args...)}
}

func Internal(format string, args ...any) *AppError {
	return &AppError{httpCode: http.StatusInternalServerError, Message: fmt.Sprintf(format, args...)}
}

func Is(err error) (*AppError, bool) {
	ae, ok := err.(*AppError)
	return ae, ok
}
