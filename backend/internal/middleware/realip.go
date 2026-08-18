package middleware

import (
	"net"
	"net/http"
	"os"
	"strings"
)

// defaultTrustedCIDRs are the networks a reverse proxy normally sits in. Coolify
// fronts the app with Traefik on a private Docker network, so the direct peer is
// always a private address. We trust forwarded headers only from these ranges.
var defaultTrustedCIDRs = []string{
	"127.0.0.0/8",    // loopback
	"::1/128",        // loopback v6
	"10.0.0.0/8",     // RFC1918
	"172.16.0.0/12",  // RFC1918 / Docker bridge
	"192.168.0.0/16", // RFC1918
	"100.64.0.0/10",  // CGNAT (some proxies)
	"fc00::/7",       // unique local v6
	"fe80::/10",      // link-local v6
}

// trustedProxyNets is the parsed trusted set, built once at startup.
var trustedProxyNets = parseTrustedProxies()

// parseTrustedProxies reads TRUSTED_PROXIES (comma-separated CIDRs) when set,
// otherwise uses the private-range defaults.
func parseTrustedProxies() []*net.IPNet {
	cidrs := defaultTrustedCIDRs
	if env := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES")); env != "" {
		cidrs = strings.Split(env, ",")
	}
	var nets []*net.IPNet
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, n, err := net.ParseCIDR(c); err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}

// isTrustedProxy reports whether an IP is a trusted reverse-proxy address.
func isTrustedProxy(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, n := range trustedProxyNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// RealIP sets r.RemoteAddr to the real client IP, but ONLY when the direct peer
// is a trusted proxy. This replaces chi's RealIP, which trusts client-supplied
// X-Forwarded-For / X-Real-IP headers unconditionally and lets an attacker spoof
// their IP to dodge every per-IP rate limit. When the peer is not trusted, the
// forwarded headers are ignored and the socket address stands.
func RealIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		peer := net.ParseIP(host)
		if isTrustedProxy(peer) {
			if client := clientIPFromHeaders(r); client != "" {
				r.RemoteAddr = net.JoinHostPort(client, "0")
			}
		}
		next.ServeHTTP(w, r)
	})
}

// clientIPFromHeaders returns the client IP the trusted proxy reported. It walks
// X-Forwarded-For from the right and returns the first address that is not
// itself a trusted proxy — the client that the proxy chain cannot forge. It
// falls back to X-Real-IP.
func clientIPFromHeaders(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			ipStr := strings.TrimSpace(parts[i])
			ip := net.ParseIP(ipStr)
			if ip == nil {
				continue
			}
			if !isTrustedProxy(ip) {
				return ipStr
			}
		}
	}
	if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
		if net.ParseIP(xr) != nil {
			return xr
		}
	}
	return ""
}
