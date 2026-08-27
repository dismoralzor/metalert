package handler

import (
	"encoding/json"
	"net/http"

	"github.com/dismoralzor/metalert/internal/model"
	"github.com/dismoralzor/metalert/internal/repository"
)

// UpdateJSONHandler обслуживает POST /update с JSON-телом.
type UpdateJSONHandler struct {
	storage repository.Storage
}

func NewUpdateJSONHandler(storage repository.Storage) *UpdateJSONHandler {
	return &UpdateJSONHandler{storage: storage}
}

func (h *UpdateJSONHandler) Update(w http.ResponseWriter, r *http.Request) {
	var m models.Metrics
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	// Как в URL-протоколе: пустое имя - 404, а не 400.
	if m.ID == "" {
		http.Error(w, "metric id is required", http.StatusNotFound)
		return
	}

	switch m.MType {
	case models.Gauge:
		if m.Value == nil {
			http.Error(w, "value is required for gauge", http.StatusBadRequest)
			return
		}
		h.storage.UpdateGauge(m.ID, *m.Value)

	case models.Counter:
		if m.Delta == nil {
			http.Error(w, "delta is required for counter", http.StatusBadRequest)
			return
		}
		h.storage.UpdateCounter(m.ID, *m.Delta)

	default:
		http.Error(w, "unknown metric type", http.StatusBadRequest)
		return
	}

	// Отдаём накопленное значение из хранилища, а не присланное тело:
	// UpdateCounter делает +=, поэтому для counter они не совпадают.
	h.fillStoredValue(&m)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(m); err != nil {
		return
	}
}
func (h *UpdateJSONHandler) fillStoredValue(m *models.Metrics) {
	switch m.MType {
	case models.Gauge:
		if value, ok := h.storage.GetGauge(m.ID); ok {
			m.Value = &value
		}
	case models.Counter:
		if delta, ok := h.storage.GetCounter(m.ID); ok {
			m.Delta = &delta
		}
	}
}
