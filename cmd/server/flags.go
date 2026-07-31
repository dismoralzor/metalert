package main

import (
	"flag"
	"log"
	"os"
	"strconv"
	"time"
)

type config struct {
	addr     string
	logLevel string
	// storeInterval == 0 означает синхронную запись после каждого обновления.
	storeInterval   time.Duration
	fileStoragePath string
	restore         bool
}

func parseFlags() config {
	addr := flag.String("a", "localhost:8080", "адрес HTTP-сервера")
	level := flag.String("l", "info", "уровень логирования")
	storeInterval := flag.Int("i", 300, "интервал сохранения в файл, сек (0 - синхронно)")
	fileStoragePath := flag.String("f", "metrics.json", "путь к файлу с метриками")
	restoreFlag := flag.Bool("r", true, "загружать метрики из файла при старте")
	flag.Parse()

	cfg := config{
		addr:            *addr,
		logLevel:        *level,
		storeInterval:   time.Duration(*storeInterval) * time.Second,
		fileStoragePath: *fileStoragePath,
		restore:         *restoreFlag,
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
	if envPath := os.Getenv("FILE_STORAGE_PATH"); envPath != "" {
		cfg.fileStoragePath = envPath
	}
	if raw := os.Getenv("STORE_INTERVAL"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil {
			log.Printf("invalid STORE_INTERVAL=%q, using flag value: %v", raw, err)
		} else {
			cfg.storeInterval = time.Duration(seconds) * time.Second
		}
	}
	if raw := os.Getenv("RESTORE"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			log.Printf("invalid RESTORE=%q, using flag value: %v", raw, err)
		} else {
			cfg.restore = parsed
		}
	}

	return cfg
}
