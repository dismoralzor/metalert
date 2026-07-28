package main

import (
	"flag"
	"log"
	"os"
	"strconv"
	"time"
)

type config struct {
	addr           string
	pollInterval   time.Duration
	reportInterval time.Duration
}

func parseFlags() config {
	addr := flag.String("a", "localhost:8080", "адрес сервера")
	// int, а не time.Duration: по заданию флаг принимает голое число секунд,
	// не Go-синтаксис вида "10s".
	reportInterval := flag.Int("r", 10, "интервал отправки метрик, сек")
	pollInterval := flag.Int("p", 2, "интервал сбора метрик, сек")
	flag.Parse()

	cfg := config{
		addr:           *addr,
		pollInterval:   time.Duration(*pollInterval) * time.Second,
		reportInterval: time.Duration(*reportInterval) * time.Second,
	}

	// Приоритет env > флаг > дефолт: флаги уже разобраны (в них дефолты),
	// а env проверяем ПОСЛЕ и перезаписываем cfg, только если переменная задана.
	if envAddr := os.Getenv("ADDRESS"); envAddr != "" {
		cfg.addr = envAddr
	}
	cfg.reportInterval = envInterval("REPORT_INTERVAL", cfg.reportInterval)
	cfg.pollInterval = envInterval("POLL_INTERVAL", cfg.pollInterval)

	return cfg
}

// envInterval возвращает интервал из переменной окружения (число секунд строкой).
// Если переменная не задана или содержит мусор - возвращает fallback (значение
// из флага), а про кривое значение пишет в лог, но не роняет программу.
func envInterval(name string, fallback time.Duration) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}

	seconds, err := strconv.Atoi(raw)
	if err != nil {
		log.Printf("invalid %s=%q, using flag value: %v", name, raw, err)
		return fallback
	}

	return time.Duration(seconds) * time.Second
}
