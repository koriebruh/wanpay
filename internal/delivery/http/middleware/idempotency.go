package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"wanpey/core/internal/infrastructure/cache"
	"wanpey/core/pkg/response"
)

const (
	idempotencyHeader = "X-Idempotency-Key"
	// 24h covers the maximum payment expiry window across all providers.
	idempotencyTTL = 24 * time.Hour
	// processingGuard is a sentinel stored while a request is in-flight.
	// Concurrent requests with the same key receive 409 instead of racing.
	processingGuard = "__processing__"
	processingTTL   = 30 * time.Second
)

type cachedResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
}

// Idempotency prevents duplicate payment processing when merchants retry on timeout.
// Redis key: "idempotency:{merchant_id}:{key}". Falls back to in-memory cache when Redis is disabled.
// Concurrent requests with the same key receive 409 Conflict — only one proceeds.
func Idempotency(c cache.Cache) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ec echo.Context) error {
			key := ec.Request().Header.Get(idempotencyHeader)
			if key == "" {
				return next(ec)
			}

			merchantID, _ := ec.Get("merchant_id").(string)
			if merchantID == "" {
				// Do not process idempotency without an authenticated identity —
				// keys from different merchants would collide in the same namespace.
				return next(ec)
			}

			redisKey := fmt.Sprintf("idempotency:%s:%s", merchantID, key)
			ctx := ec.Request().Context()

			// Attempt to claim this key atomically. SetNX returns true only for the
			// first caller — concurrent duplicates see the processing guard below.
			claimed, err := c.SetNX(ctx, redisKey, []byte(processingGuard), processingTTL)
			if err != nil {
				return next(ec)
			}

			if !claimed {
				val, getErr := c.Get(ctx, redisKey)
				if getErr != nil {
					return next(ec)
				}
				if string(val) == processingGuard {
					return response.Err(ec, http.StatusConflict,
						"a request with this idempotency key is already being processed",
					)
				}
				var resp cachedResponse
				if jsonErr := json.Unmarshal(val, &resp); jsonErr == nil {
					for k, v := range resp.Headers {
						ec.Response().Header().Set(k, v)
					}
					ec.Response().Header().Set("X-Idempotency-Replayed", "true")
					return ec.JSONBlob(resp.StatusCode, resp.Body)
				}
				return next(ec)
			}

			rec := newBodyCapture(ec.Response().Writer)
			ec.Response().Writer = rec

			handlerErr := next(ec)
			status := ec.Response().Status

			if handlerErr != nil || status >= 500 {
				// Delete claim so the client can retry fresh.
				_ = c.Del(ctx, redisKey)
				return handlerErr
			}

			resp := cachedResponse{
				StatusCode: status,
				Headers:    extractSafeHeaders(ec.Response().Header()),
				Body:       rec.Body(),
			}
			if b, jsonErr := json.Marshal(resp); jsonErr == nil {
				_ = c.Set(ctx, redisKey, b, idempotencyTTL)
			}

			return nil
		}
	}
}

// bodyCapture wraps http.ResponseWriter to capture the response body for caching.
// Implements http.Flusher to avoid breaking streaming handlers upstream.
type bodyCapture struct {
	http.ResponseWriter
	buf *bytes.Buffer
}

func newBodyCapture(w http.ResponseWriter) *bodyCapture {
	return &bodyCapture{ResponseWriter: w, buf: &bytes.Buffer{}}
}

func (b *bodyCapture) Write(p []byte) (int, error) {
	b.buf.Write(p)
	return b.ResponseWriter.Write(p)
}

func (b *bodyCapture) Body() []byte {
	return b.buf.Bytes()
}

func (b *bodyCapture) Flush() {
	if f, ok := b.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func extractSafeHeaders(h http.Header) map[string]string {
	keys := []string{"Content-Type", echo.HeaderXRequestID}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		if v := h.Get(k); v != "" {
			out[k] = v
		}
	}
	return out
}
