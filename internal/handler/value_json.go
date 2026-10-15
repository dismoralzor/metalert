package handler

import (
	"encoding/json"
	"net/http"

	"github.com/dismoralzor/metalert/internal/model"
	"github.com/dismoralzor/metalert/internal/repository"
)

// ValueJSONHandler обслуживает POST /value: принимает id и type в JSON-теле,
// возвращает ту же структуру с заполненным Delta или Value.
type ValueJSONHandler struct {
	storage repository.Storage
}

func NewValueJSONHandler(storage repository.Storage) *ValueJSONHandler {
	return &ValueJSONHandler{storage: storage}
}

func (h *ValueJSONHandler) Value(w http.ResponseWriter, r *http.Request) {
	var m models.Metrics
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	switch m.MType {
	case models.Gauge:
		value, ok := h.storage.GetGauge(r.Context(), m.ID)
		if !ok {
			http.Error(w, "metric not found", http.StatusNotFound)
			return
		}
		m.Value = &value

	case models.Counter:
		delta, ok := h.storage.GetCounter(r.Context(), m.ID)
		if !ok {
			http.Error(w, "metric not found", http.StatusNotFound)
			return
		}
		m.Delta = &delta

	default:
		// Как в GET /value: неизвестный тип - "метрики нет", 404, а не 400.
		http.Error(w, "unknown metric type", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(m); err != nil {
		return
	}
}
