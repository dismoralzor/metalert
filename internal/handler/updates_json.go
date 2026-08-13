package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/dismoralzor/metalert/internal/model"
	"github.com/dismoralzor/metalert/internal/repository"
)

// UpdatesJSONHandler обслуживает POST /updates - приём метрик одним батчем
// вместо N отдельных запросов на /update, как раньше слал агент.
type UpdatesJSONHandler struct {
	storage repository.Storage
}

func NewUpdatesJSONHandler(storage repository.Storage) *UpdatesJSONHandler {
	return &UpdatesJSONHandler{storage: storage}
}

func (h *UpdatesJSONHandler) Update(w http.ResponseWriter, r *http.Request) {
	var metrics []models.Metrics
	if err := json.NewDecoder(r.Body).Decode(&metrics); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	if len(metrics) == 0 {
		http.Error(w, "empty batch", http.StatusBadRequest)
		return
	}

	// Валидируем весь батч ДО записи: частично применённый батч (первые N метрик
	// прошли, N+1-я невалидна) хуже, чем явный отказ целиком.
	for _, m := range metrics {
		if err := validateMetric(m); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	if err := h.storage.UpdateBatch(r.Context(), metrics); err != nil {
		http.Error(w, "failed to save metrics", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

func validateMetric(m models.Metrics) error {
	if m.ID == "" {
		return errors.New("metric id is required")
	}

	switch m.MType {
	case models.Gauge:
		if m.Value == nil {
			return fmt.Errorf("value is required for gauge %q", m.ID)
		}
	case models.Counter:
		if m.Delta == nil {
			return fmt.Errorf("delta is required for counter %q", m.ID)
		}
	default:
		return fmt.Errorf("unknown metric type %q", m.MType)
	}

	return nil
}
