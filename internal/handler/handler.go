package handler

import (
	"net/http"
	"strconv"

	"github.com/dismoralzor/metalert/internal/model"
	"github.com/dismoralzor/metalert/internal/repository"
)

// Хранит только интерфейс repository.Storage, а не конкретный MemStorage -
// это позволяет в тестах подставить фейковую реализацию Storage.
type UpdateHandler struct {
	storage repository.Storage
}

func NewUpdateHandler(storage repository.Storage) *UpdateHandler {
	return &UpdateHandler{storage: storage}
}

func (h *UpdateHandler) Update(w http.ResponseWriter, r *http.Request) {
	metricType := r.PathValue("type")
	metricName := r.PathValue("name")
	rawValue := r.PathValue("value")

	// Сначала проверяем имя: по спецификации отсутствие имени - это отдельный
	// случай (404), который не должен смешиваться с "кривой тип/значение" (400).
	if metricName == "" {
		http.Error(w, "metric name is required", http.StatusNotFound)
		return
	}

	switch metricType {
	case models.Gauge:
		value, err := strconv.ParseFloat(rawValue, 64)
		if err != nil {
			http.Error(w, "invalid gauge value", http.StatusBadRequest)
			return
		}
		h.storage.UpdateGauge(metricName, value)

	case models.Counter:
		delta, err := strconv.ParseInt(rawValue, 10, 64)
		if err != nil {
			http.Error(w, "invalid counter value", http.StatusBadRequest)
			return
		}
		h.storage.UpdateCounter(metricName, delta)

	default:
		http.Error(w, "unknown metric type", http.StatusBadRequest)
		return
	}

	// http.Error сам проставляет Content-Type на ошибочных ответах,
	// а для успешного ответа выставляем его явно, как требует спецификация.
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
}