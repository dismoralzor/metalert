// Package handler содержит HTTP-обработчики сервера метрик: приём метрик
// (текстовый и JSON-протоколы, одиночные и батчем), выдачу значений, страницу
// со списком метрик и проверку связи с БД. Обработчики зависят от интерфейса
// repository.Storage, что позволяет подменять хранилище в тестах.
package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dismoralzor/metalert/internal/audit"
	models "github.com/dismoralzor/metalert/internal/model"
	"github.com/dismoralzor/metalert/internal/repository"
)

// UpdateHandler обслуживает POST /update/{type}/{name}/{value} (текстовый
// протокол). Хранит только интерфейс repository.Storage, а не конкретный
// MemStorage - это позволяет в тестах подставить фейковую реализацию Storage.
type UpdateHandler struct {
	storage repository.Storage
	auditor Auditor
}

// NewUpdateHandler создаёт обработчик текстового /update поверх хранилища
// storage; auditor может быть nil - тогда аудит не ведётся.
func NewUpdateHandler(storage repository.Storage, auditor Auditor) *UpdateHandler {
	return &UpdateHandler{storage: storage, auditor: auditor}
}

// Update принимает одну метрику из пути URL, сохраняет её и возвращает 200.
// Пустое имя - 404, неизвестный тип или неразборное значение - 400.
func (h *UpdateHandler) Update(w http.ResponseWriter, r *http.Request) {
	metricType := chi.URLParam(r, "type")
	metricName := chi.URLParam(r, "name")
	rawValue := chi.URLParam(r, "value")

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
		h.storage.UpdateGauge(r.Context(), metricName, value)

	case models.Counter:
		delta, err := strconv.ParseInt(rawValue, 10, 64)
		if err != nil {
			http.Error(w, "invalid counter value", http.StatusBadRequest)
			return
		}
		h.storage.UpdateCounter(r.Context(), metricName, delta)

	default:
		http.Error(w, "unknown metric type", http.StatusBadRequest)
		return
	}

	// Аудит ДО ответа 200: к моменту, когда клиент видит успех, событие уже
	// разослано приёмникам (для файла - строка уже на диске).
	if h.auditor != nil {
		h.auditor.NotifyAll(audit.Event{
			TS:        time.Now().Unix(),
			Metrics:   []string{metricName},
			IPAddress: clientIP(r),
		})
	}

	// http.Error сам проставляет Content-Type на ошибочных ответах,
	// а для успешного ответа выставляем его явно, как требует спецификация.
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
}
