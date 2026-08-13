package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/dismoralzor/metalert/internal/model"
	"github.com/dismoralzor/metalert/internal/repository"
)

// ValueHandler обслуживает GET /value/{type}/{name}.
type ValueHandler struct {
	storage repository.Storage
}

func NewValueHandler(storage repository.Storage) *ValueHandler {
	return &ValueHandler{storage: storage}
}

func (h *ValueHandler) Value(w http.ResponseWriter, r *http.Request) {
	metricType := chi.URLParam(r, "type")
	metricName := chi.URLParam(r, "name")

	var value string

	switch metricType {
	case models.Gauge:
		v, ok := h.storage.GetGauge(metricName)
		if !ok {
			http.Error(w, "metric not found", http.StatusNotFound)
			return
		}
		value = strconv.FormatFloat(v, 'f', -1, 64)

	case models.Counter:
		v, ok := h.storage.GetCounter(metricName)
		if !ok {
			http.Error(w, "metric not found", http.StatusNotFound)
			return
		}
		value = strconv.FormatInt(v, 10)

	default:
		// В отличие от Update (400), тут неизвестный тип - это "метрика не найдена".
		http.Error(w, "unknown metric type", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(value))
}
