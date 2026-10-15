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

func NewPingHandler(db *sql.DB) *PingHandler {
	return &PingHandler{db: db}
}

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
