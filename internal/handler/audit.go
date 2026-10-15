package handler

import (
	"net"
	"net/http"
	"strings"

	"github.com/dismoralzor/metalert/internal/audit"
)

// Auditor - зависимость хендлеров для аудита принятых метрик. Интерфейс (а не
// конкретный *audit.Subject), как и storage: в тестах подставляется фейк,
// а при выключенном аудите поле остаётся nil и NotifyAll просто не вызывается.
type Auditor interface {
	NotifyAll(audit.Event)
}

// clientIP определяет IP клиента с учётом прокси:
// X-Real-IP -> первый адрес X-Forwarded-For -> host из RemoteAddr.
func clientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}

	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		// X-Forwarded-For: "client, proxy1, proxy2" - клиент идёт первым.
		if first := strings.TrimSpace(strings.Split(fwd, ",")[0]); first != "" {
			return first
		}
	}

	// RemoteAddr - это "host:port"; порт отбрасываем. Если порта нет
	// (нестандартный транспорт/тесты), возвращаем RemoteAddr как есть.
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
