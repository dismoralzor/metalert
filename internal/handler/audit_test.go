package handler

import (
	"net/http"
	"testing"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		realIP     string
		forwarded  string
		remoteAddr string
		want       string
	}{
		{
			name:       "X-Real-IP wins over everything",
			realIP:     "10.0.0.1",
			forwarded:  "20.0.0.1, 20.0.0.2",
			remoteAddr: "30.0.0.1:12345",
			want:       "10.0.0.1",
		},
		{
			name:       "first address of X-Forwarded-For when no X-Real-IP",
			forwarded:  "20.0.0.1, 20.0.0.2",
			remoteAddr: "30.0.0.1:12345",
			want:       "20.0.0.1",
		},
		{
			name:       "RemoteAddr host when no proxy headers",
			remoteAddr: "30.0.0.1:12345",
			want:       "30.0.0.1",
		},
		{
			name:       "RemoteAddr without port returned as-is",
			remoteAddr: "30.0.0.1",
			want:       "30.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{
				Header:     http.Header{},
				RemoteAddr: tt.remoteAddr,
			}
			if tt.realIP != "" {
				r.Header.Set("X-Real-IP", tt.realIP)
			}
			if tt.forwarded != "" {
				r.Header.Set("X-Forwarded-For", tt.forwarded)
			}

			if got := clientIP(r); got != tt.want {
				t.Errorf("clientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}
