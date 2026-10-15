package handler

import (
	"context"
	"database/sql"
	"net/http"
	"time"
)

// PingHandler обслуживает GET /ping - проверку связи с БД.
type PingHandler struct {
	db *sql.DB
}

// NewPingHandler создаёт обработчик GET /ping поверх подключения db
// (db может быть nil, если БД не сконфигурирована).
func NewPingHandler(db *sql.DB) *PingHandler {
	return &PingHandler{db: db}
}

// Ping отвечает 200, если БД доступна, и 500 - если БД не настроена или
// недоступна.
func (h *PingHandler) Ping(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		http.Error(w, "database is not configured", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()

	if err := h.db.PingContext(ctx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
