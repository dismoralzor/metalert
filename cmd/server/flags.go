package main

import (
	"flag"
	"os"
)

type config struct {
	addr     string
	logLevel string
}

func parseFlags() config {
	addr := flag.String("a", "localhost:8080", "адрес HTTP-сервера")
	level := flag.String("l", "info", "уровень логирования")
	flag.Parse()

	cfg := config{
		addr:     *addr,
		logLevel: *level,
	}

	// Приоритет env > флаг > дефолт: флаги уже разобраны (в них дефолты),
	// а env проверяем ПОСЛЕ и перезаписываем cfg, только если переменная задана
	// и непустая - иначе пустой ADDRESS затёр бы валидный флаг.
	if envAddr := os.Getenv("ADDRESS"); envAddr != "" {
		cfg.addr = envAddr
	}
	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		cfg.logLevel = envLevel
	}

	return cfg
}
