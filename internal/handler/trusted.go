package handler

import (
	"net"
	"net/http"
)

// TrustedSubnetMiddleware пропускает запросы только от клиентов из доверенной
// подсети: IP берётся из заголовка X-Real-IP, его и проверяем на вхождение в
// subnet. subnet == nil (опция trusted_subnet не задана) - middleware прозрачен
// и ограничений нет. Отсутствующий или кривой IP при заданной подсети - 403.
func TrustedSubnetMiddleware(subnet *net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if subnet != nil {
				ip := net.ParseIP(r.Header.Get("X-Real-IP"))
				if ip == nil || !subnet.Contains(ip) {
					http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
