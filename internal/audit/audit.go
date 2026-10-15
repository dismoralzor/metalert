// Package audit реализует паттерн Наблюдатель для аудита запросов: сервер
// формирует событие о принятых метриках (Subject) и синхронно рассылает его
// всем подключённым приёмникам (Observer) - файлу, HTTP-эндпоинту и т.п.
package audit

import (
	"sync"

	"go.uber.org/zap"

	"github.com/dismoralzor/metalert/internal/logger"
)

// Event - событие аудита: что именно сервер принял и от кого.
// JSON-теги фиксированы форматом приёмников (одна JSONL-строка на событие).
type Event struct {
	TS        int64    `json:"ts"`         // время приёма, Unix-секунды
	Metrics   []string `json:"metrics"`    // имена принятых метрик
	IPAddress string   `json:"ip_address"` // IP клиента
}

// Observer - приёмник событий аудита. Реализации: FileObserver, HTTPObserver.
type Observer interface {
	Notify(Event) error
}

// Subject - издатель: хранит подписчиков и рассылает им события.
// Доступ к срезу под RWMutex: NotifyAll читает его под RLock (параллельные
// запросы рассылают одновременно), Subscribe пишет под Lock.
type Subject struct {
	mu        sync.RWMutex
	observers []Observer
}

// NewSubject создаёт пустой издатель без подписчиков.
func NewSubject() *Subject {
	return &Subject{}
}

// Subscribe добавляет приёмник. Вызывается при старте сервера, до приёма
// запросов, но Lock держим из принципа - срез мутируется.
func (s *Subject) Subscribe(o Observer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observers = append(s.observers, o)
}

// NotifyAll синхронно рассылает событие всем подписчикам. Ошибку отдельного
// приёмника логируем и продолжаем обход: сбой аудита не должен ни ронять
// запрос, ни мешать остальным приёмникам получить событие.
func (s *Subject) NotifyAll(e Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, o := range s.observers {
		if err := o.Notify(e); err != nil {
			logger.Log.Error("audit observer failed", zap.Error(err))
		}
	}
}
