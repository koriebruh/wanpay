package middleware

import (
	"net"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"wanpey/core/pkg/apperror"
)

// WebhookIPAllowlist returns middleware that restricts webhook endpoints by source IP.
// allowedByProvider maps provider name → list of allowed CIDRs or exact IPs.
// If a provider has no entry (or empty list), all IPs are accepted for that provider.
// Always use c.RealIP() — Echo resolves X-Real-IP / X-Forwarded-For respecting TrustedProxies.
func WebhookIPAllowlist(allowedByProvider map[string][]string, log *zap.Logger) echo.MiddlewareFunc {
	// Pre-parse CIDRs at middleware construction time to avoid per-request parsing.
	parsed := make(map[string][]*net.IPNet, len(allowedByProvider))
	for provider, cidrs := range allowedByProvider {
		nets := make([]*net.IPNet, 0, len(cidrs))
		for _, cidr := range cidrs {
			// Accept plain IPs as /32 or /128.
			if !containsSlash(cidr) {
				if ip := net.ParseIP(cidr); ip != nil {
					if ip.To4() != nil {
						cidr += "/32"
					} else {
						cidr += "/128"
					}
				}
			}
			_, ipnet, err := net.ParseCIDR(cidr)
			if err != nil {
				log.Warn("invalid CIDR in webhook allowlist — skipping",
					zap.String("provider", provider),
					zap.String("cidr", cidr),
					zap.Error(err),
				)
				continue
			}
			nets = append(nets, ipnet)
		}
		if len(nets) > 0 {
			parsed[provider] = nets
		}
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			provider := c.Param("provider")
			nets, ok := parsed[provider]
			if !ok || len(nets) == 0 {
				return next(c) // no restriction configured for this provider
			}

			ipStr := c.RealIP()
			ip := net.ParseIP(ipStr)
			if ip == nil {
				log.Warn("webhook: cannot parse source IP",
					zap.String("provider", provider),
					zap.String("ip", ipStr),
				)
				return apperror.Forbidden("source IP not permitted")
			}

			for _, n := range nets {
				if n.Contains(ip) {
					return next(c)
				}
			}

			log.Warn("webhook: IP not in allowlist — rejecting",
				zap.String("provider", provider),
				zap.String("ip", ipStr),
			)
			return apperror.Forbidden("source IP not permitted")
		}
	}
}

func containsSlash(s string) bool {
	for _, ch := range s {
		if ch == '/' {
			return true
		}
	}
	return false
}
