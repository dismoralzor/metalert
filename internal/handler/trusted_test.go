package handler

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrustedSubnetMiddleware(t *testing.T) {
	_, subnet, err := net.ParseCIDR("192.168.1.0/24")
	if err != nil {
		t.Fatalf("ParseCIDR: %v", err)
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name   string
		subnet *net.IPNet
		realIP string
		want   int
	}{
		{"подсеть не задана - без ограничений", nil, "", http.StatusOK},
		{"IP из подсети - пропускаем", subnet, "192.168.1.42", http.StatusOK},
		{"IP вне подсети - 403", subnet, "10.0.0.1", http.StatusForbidden},
		{"заголовок отсутствует - 403", subnet, "", http.StatusForbidden},
		{"кривой IP - 403", subnet, "not-an-ip", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/updates/", nil)
			if tt.realIP != "" {
				req.Header.Set("X-Real-IP", tt.realIP)
			}
			rec := httptest.NewRecorder()

			TrustedSubnetMiddleware(tt.subnet)(next).ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}
