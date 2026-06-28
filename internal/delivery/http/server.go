package http

import (
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/samber/do/v2"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.uber.org/zap"

	"wanpey/core/internal/infrastructure/config"
	"wanpey/core/pkg/apperror"
	"wanpey/core/pkg/response"
	"wanpey/core/pkg/validator"
)

func ProvideEcho(i do.Injector) {
	do.Provide(i, func(i do.Injector) (*echo.Echo, error) {
		cfg := do.MustInvoke[*config.Config](i)
		log := do.MustInvoke[*zap.Logger](i)
		return buildEcho(cfg, log), nil
	})
}

func buildEcho(cfg *config.Config, log *zap.Logger) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	e.Validator = validator.New()

	e.Use(middleware.Recover())
	e.Use(otelecho.Middleware(cfg.App.Name))

	// Propagate request ID to context so usecase/repository layers can include it in logs.
	e.Use(middleware.RequestIDWithConfig(middleware.RequestIDConfig{
		RequestIDHandler: func(c echo.Context, id string) {
			c.Set("request_id", id)
		},
	}))

	// Reject oversized request bodies before they hit handlers.
	bodyLimit := cfg.App.HTTP.MaxBodySize
	if bodyLimit == "" {
		bodyLimit = "1M"
	}
	e.Use(middleware.BodyLimit(bodyLimit))

	reqTimeout := time.Duration(cfg.App.HTTP.RequestTimeoutSeconds) * time.Second
	if reqTimeout <= 0 || reqTimeout > 30*time.Second {
		reqTimeout = 30 * time.Second
	}
	e.Use(middleware.ContextTimeoutWithConfig(middleware.ContextTimeoutConfig{
		Timeout: reqTimeout,
	}))

	e.Use(requestLogger(log))

	origins := cfg.App.HTTP.CORSAllowOrigins
	if len(origins) == 0 {
		origins = []string{"http://localhost:3000"}
	}
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: origins,
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowHeaders: []string{
			echo.HeaderContentType,
			echo.HeaderAccept,
			"X-API-Key",
			"X-Idempotency-Key",
			echo.HeaderAuthorization,
		},
		ExposeHeaders: []string{
			echo.HeaderXRequestID, // allow frontend to read request ID for error reporting
		},
		AllowCredentials: false, // API key auth — cookies not used
		MaxAge:           86400, // preflight cache: 24h
	}))

	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		Skipper: func(c echo.Context) bool {
			return len(c.Path()) >= 8 && c.Path()[:8] == "/swagger"
		},
		XSSProtection:         "1; mode=block",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		HSTSMaxAge:            31536000,
		ContentSecurityPolicy: "default-src 'self'",
	}))

	e.HTTPErrorHandler = globalErrorHandler(log, cfg.App.Env)

	return e
}

func requestLogger(log *zap.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			latency := time.Since(start)

			status := c.Response().Status
			fields := []zap.Field{
				zap.String("request_id", c.Response().Header().Get(echo.HeaderXRequestID)),
				zap.String("method", c.Request().Method),
				zap.String("path", c.Request().URL.Path),
				zap.Int("status", status),
				zap.Duration("latency", latency),
				zap.String("ip", c.RealIP()),
			}
			if merchantID, ok := c.Get("merchant_id").(string); ok && merchantID != "" {
				fields = append(fields, zap.String("merchant_id", merchantID))
			}
			if adminID, ok := c.Get("admin_id").(string); ok && adminID != "" {
				fields = append(fields, zap.String("admin_id", adminID))
			}

			if status >= 500 {
				log.Error("request", fields...)
			} else if status >= 400 {
				log.Warn("request", fields...)
			} else {
				log.Info("request", fields...)
			}

			return err
		}
	}
}

func globalErrorHandler(log *zap.Logger, env string) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}

		requestID := c.Response().Header().Get(echo.HeaderXRequestID)

		status := http.StatusInternalServerError
		message := "internal server error"
		var details []response.FieldDetail

		var ae *apperror.AppError
		if errors.As(err, &ae) {
			status = ae.HTTPCode()
			message = ae.Message
			for _, d := range ae.Details {
				details = append(details, response.FieldDetail{Field: d.Field, Message: d.Message})
			}
		} else if he, ok := err.(*echo.HTTPError); ok {
			status = he.Code
			if m, ok := he.Message.(string); ok {
				message = m
			}
		}

		if status >= 500 {
			log.Error("unhandled error",
				zap.String("request_id", requestID),
				zap.String("path", c.Request().URL.Path),
				zap.Error(err),
			)
			if env == "production" {
				message = "internal server error"
				details = nil
			}
		}

		if respErr := response.Err(c, status, message, details...); respErr != nil {
			log.Error("failed to write error response",
				zap.String("request_id", requestID),
				zap.Error(respErr),
			)
		}
	}
}
