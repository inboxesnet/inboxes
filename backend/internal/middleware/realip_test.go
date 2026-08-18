package middleware

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRealIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		xRealIP    string
		wantHost   string // expected host portion of RemoteAddr after the middleware
	}{
		{
			name:       "untrusted peer cannot spoof via XFF",
			remoteAddr: "8.8.8.8:4000",
			xff:        "1.2.3.4",
			wantHost:   "8.8.8.8",
		},
		{
			name:       "untrusted peer cannot spoof via X-Real-IP",
			remoteAddr: "8.8.8.8:4000",
			xRealIP:    "1.2.3.4",
			wantHost:   "8.8.8.8",
		},
		{
			name:       "trusted proxy sets the client IP",
			remoteAddr: "10.0.0.5:5000",
			xff:        "1.2.3.4",
			wantHost:   "1.2.3.4",
		},
		{
			name:       "trusted proxy: rightmost untrusted hop wins over injected left value",
			remoteAddr: "172.18.0.2:5000",
			xff:        "5.5.5.5, 1.1.1.1",
			wantHost:   "1.1.1.1",
		},
		{
			name:       "trusted proxy: skip trailing trusted hops",
			remoteAddr: "10.1.2.3:5000",
			xff:        "9.9.9.9, 10.0.0.9",
			wantHost:   "9.9.9.9",
		},
		{
			name:       "trusted proxy falls back to X-Real-IP",
			remoteAddr: "192.168.1.1:5000",
			xRealIP:    "1.2.3.4",
			wantHost:   "1.2.3.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotHost string
			h := RealIP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				host, _, err := net.SplitHostPort(r.RemoteAddr)
				if err != nil {
					host = r.RemoteAddr
				}
				gotHost = host
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}
			h.ServeHTTP(httptest.NewRecorder(), req)

			if gotHost != tt.wantHost {
				t.Errorf("RemoteAddr host = %q, want %q", gotHost, tt.wantHost)
			}
		})
	}
}
