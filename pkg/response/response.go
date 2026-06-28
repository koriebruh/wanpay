package response

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/trace"
)

type Meta struct {
	TraceID    string      `json:"trace_id,omitempty"`
	Timestamp  string      `json:"timestamp"`
	Pagination *Pagination `json:"pagination,omitempty"`
}

type Pagination struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type ErrorBody struct {
	Message string        `json:"message"`
	Details []FieldDetail `json:"details,omitempty"`
}

type FieldDetail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type envelope struct {
	Success bool       `json:"success"`
	Data    any        `json:"data,omitempty"`
	Error   *ErrorBody `json:"error,omitempty"`
	Meta    Meta       `json:"meta"`
}

// SuccessResponse is the exported shape used by Swagger annotations.
type SuccessResponse struct {
	Success bool `json:"success" example:"true"`
	Data    any  `json:"data"`
	Meta    Meta `json:"meta"`
}

// ErrorResponse is the exported shape used by Swagger annotations.
type ErrorResponse struct {
	Success bool       `json:"success" example:"false"`
	Error   *ErrorBody `json:"error"`
	Meta    Meta       `json:"meta"`
}

// ListResponse is the exported shape for paginated list endpoints.
type ListResponse struct {
	Success bool `json:"success" example:"true"`
	Data    any  `json:"data"`
	Meta    Meta `json:"meta"`
}

func meta(c echo.Context) Meta {
	m := Meta{Timestamp: time.Now().UTC().Format(time.RFC3339Nano)}
	if span := trace.SpanFromContext(c.Request().Context()); span.SpanContext().IsValid() {
		m.TraceID = span.SpanContext().TraceID().String()
	}
	return m
}

func OK(c echo.Context, data any) error {
	return c.JSON(http.StatusOK, envelope{Success: true, Data: data, Meta: meta(c)})
}

func Created(c echo.Context, data any) error {
	return c.JSON(http.StatusCreated, envelope{Success: true, Data: data, Meta: meta(c)})
}

func List(c echo.Context, data any, p Pagination) error {
	m := meta(c)
	m.Pagination = &p
	return c.JSON(http.StatusOK, envelope{Success: true, Data: data, Meta: m})
}

func NoContent(c echo.Context) error {
	return c.NoContent(http.StatusNoContent)
}

func Err(c echo.Context, status int, message string, details ...FieldDetail) error {
	body := &ErrorBody{Message: message}
	if len(details) > 0 {
		body.Details = details
	}
	return c.JSON(status, envelope{Success: false, Error: body, Meta: meta(c)})
}
