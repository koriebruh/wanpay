package http

import "github.com/labstack/echo/v4"

// V1Group returns the /v1 route group.
// All payment API handlers must be registered under this group.
func V1Group(e *echo.Echo) *echo.Group {
	return e.Group("/v1")
}
