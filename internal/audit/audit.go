// Package audit реализует паттерн Наблюдатель для аудита запросов: сервер
// формирует событие о принятых метриках (Subject) и синхронно рассылает его
// всем подключённым приёмникам (Observer) - файлу, HTTP-эндпоинту и т.п.
package audit

import (
	"errors"
	"io"
	"slices"
	"sync"

	"go.uber.org/zap"
)

// Event - событие аудита: что именно сервер принял и от кого.
// JSON-теги фиксированы форматом приёмников (одна JSONL-строка на событие).
type Event struct {
	TS        int64    `json:"ts"`         // время приёма, Unix-секунды
	Metrics   []string `json:"metrics"`    // имена принятых метрик
	IPAddress string   `json:"ip_address"` // IP клиента
}

// Observer - приёмник событий аудита. Реализации: FileObserver, HTTPObserver.
// Приёмник, которому нужно освобождать ресурсы (открытый файл, пул соединений),
// дополнительно реализует io.Closer - его закроет Subject.Close.
type Observer interface {
	Notify(Event) error
}

// Subject - издатель: хранит подписчиков и рассылает им события.
// RWMutex защищает срез подписчиков, но НЕ доставку: саму рассылку ведём по
// снятой под локом копии, чтобы медленный приёмник (HTTP с таймаутом) не держал
// лок и не блокировал параллельные запросы.
type Subject struct {
	mu        sync.RWMutex
	observers []Observer
	log       *zap.Logger
}

// NewSubject создаёт пустой издатель без подписчиков. log используется для
// сообщений об ошибках приёмников.
func NewSubject(log *zap.Logger) *Subject {
	return &Subject{log: log}
}

// Subscribe добавляет приёмник. Вызывается при старте сервера, до приёма
// запросов, но Lock держим из принципа - срез мутируется.
func (s *Subject) Subscribe(o Observer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observers = append(s.observers, o)
}

// NotifyAll синхронно рассылает событие всем подписчикам. Срез подписчиков
// копируем под RLock и тут же отпускаем лок - доставку ведём уже по копии, без
// удержания лока (иначе зависший на таймауте HTTP-приёмник блокировал бы все
// запросы). Ошибку отдельного приёмника логируем и продолжаем обход: сбой аудита
// не должен ни ронять запрос, ни мешать остальным приёмникам.
func (s *Subject) NotifyAll(e Event) {
	s.mu.RLock()
	observers := slices.Clone(s.observers)
	s.mu.RUnlock()

	for _, o := range observers {
		if err := o.Notify(e); err != nil {
			s.log.Error("audit observer failed", zap.Error(err))
		}
	}
}

// Close закрывает подписчиков, реализующих io.Closer (например, FileObserver
// освобождает открытый файл). Вызывается при остановке сервера. Ошибки всех
// приёмников объединяются, чтобы одна не скрыла остальные.
func (s *Subject) Close() error {
	s.mu.RLock()
	observers := slices.Clone(s.observers)
	s.mu.RUnlock()

	var errs []error
	for _, o := range observers {
		if c, ok := o.(io.Closer); ok {
			if err := c.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
