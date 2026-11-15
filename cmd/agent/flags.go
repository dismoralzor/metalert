package main

import (
	"flag"
	"log"
	"strconv"
	"time"

	filecfg "github.com/dismoralzor/metalert/internal/config"
)

type config struct {
	addr           string
	pollInterval   time.Duration
	reportInterval time.Duration
	key            string
	// cryptoKey - путь к файлу с публичным RSA-ключом. Пусто = шифрование выключено.
	cryptoKey string
	// rateLimit - число одновременных исходящих запросов (размер воркер-пула).
	// Не путать с -l сервера (там это уровень логирования) - разные бинарники.
	rateLimit int
}

// loadConfig собирает конфигурацию слоями по нарастанию приоритета:
// дефолты -> JSON-файл (путь из -c/-config или CONFIG) -> флаги (только
// реально переданные) -> переменные окружения (только реально заданные).
// lookupEnv вынесен в параметр ради тестов, в проде это os.LookupEnv.
func loadConfig(args []string, lookupEnv func(string) (string, bool)) (config, error) {
	fs := flag.NewFlagSet("agent", flag.ContinueOnError)
	addr := fs.String("a", "localhost:8080", "адрес сервера")
	// int, а не time.Duration: по заданию флаг принимает голое число секунд,
	// не Go-синтаксис вида "10s".
	reportInterval := fs.Int("r", 10, "интервал отправки метрик, сек")
	pollInterval := fs.Int("p", 2, "интервал сбора метрик, сек")
	key := fs.String("k", "", "ключ для подписи запросов HashSHA256 (пусто - подпись выключена)")
	cryptoKey := fs.String("crypto-key", "", "путь к файлу с публичным RSA-ключом (пусто - шифрование выключено)")
	rateLimit := fs.Int("l", 1, "количество одновременных исходящих запросов")
	// -c и -config - один параметр под двумя именами.
	cfgPath := fs.String("c", "", "путь к JSON-файлу конфигурации")
	fs.StringVar(cfgPath, "config", "", "путь к JSON-файлу конфигурации")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	// Слой дефолтов.
	cfg := config{
		addr:           *addr,
		pollInterval:   time.Duration(*pollInterval) * time.Second,
		reportInterval: time.Duration(*reportInterval) * time.Second,
		key:            *key,
		cryptoKey:      *cryptoKey,
		rateLimit:      *rateLimit,
	}

	// Путь к конфигу: env CONFIG перекрывает флаг, как у остальных опций.
	path := *cfgPath
	if envPath, ok := lookupEnv("CONFIG"); ok && envPath != "" {
		path = envPath
	}

	// Слой файла: применяем только поля, реально присутствующие в JSON.
	if path != "" {
		var fc filecfg.AgentFile
		if err := filecfg.LoadJSON(path, &fc); err != nil {
			return config{}, err
		}
		if fc.Address != nil {
			cfg.addr = *fc.Address
		}
		if fc.ReportInterval != nil {
			cfg.reportInterval = fc.ReportInterval.Duration
		}
		if fc.PollInterval != nil {
			cfg.pollInterval = fc.PollInterval.Duration
		}
		if fc.Key != nil {
			cfg.key = *fc.Key
		}
		if fc.CryptoKey != nil {
			cfg.cryptoKey = *fc.CryptoKey
		}
		if fc.RateLimit != nil {
			cfg.rateLimit = *fc.RateLimit
		}
	}

	// Слой флагов: применяем только реально переданные - сравнение с дефолтом
	// не отличило бы "-a localhost:8080" от "флаг не задан".
	visited := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	if visited["a"] {
		cfg.addr = *addr
	}
	if visited["r"] {
		cfg.reportInterval = time.Duration(*reportInterval) * time.Second
	}
	if visited["p"] {
		cfg.pollInterval = time.Duration(*pollInterval) * time.Second
	}
	if visited["k"] {
		cfg.key = *key
	}
	if visited["crypto-key"] {
		cfg.cryptoKey = *cryptoKey
	}
	if visited["l"] {
		cfg.rateLimit = *rateLimit
	}

	// Слой env: реально заданная переменная перекрывает всё остальное.
	if v, ok := lookupEnv("ADDRESS"); ok && v != "" {
		cfg.addr = v
	}
	cfg.reportInterval = envInterval(lookupEnv, "REPORT_INTERVAL", cfg.reportInterval)
	cfg.pollInterval = envInterval(lookupEnv, "POLL_INTERVAL", cfg.pollInterval)
	if v, ok := lookupEnv("KEY"); ok && v != "" {
		cfg.key = v
	}
	if v, ok := lookupEnv("CRYPTO_KEY"); ok && v != "" {
		cfg.cryptoKey = v
	}
	if raw, ok := lookupEnv("RATE_LIMIT"); ok && raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			log.Printf("invalid RATE_LIMIT=%q, keeping previous value: %v", raw, err)
		} else {
			cfg.rateLimit = n
		}
	}

	// Ноль или отрицательное число воркеров означало бы, что jobsCh никто
	// никогда не читает - аккумулятор зависнет на первой же отправке в jobsCh.
	if cfg.rateLimit < 1 {
		cfg.rateLimit = 1
	}

	return cfg, nil
}

// envInterval возвращает интервал из переменной окружения (число секунд строкой).
// Если переменная не задана или содержит мусор - возвращает fallback (значение
// из более приоритетных слоёв), а про кривое значение пишет в лог.
func envInterval(lookupEnv func(string) (string, bool), name string, fallback time.Duration) time.Duration {
	raw, ok := lookupEnv(name)
	if !ok || raw == "" {
		return fallback
	}

	seconds, err := strconv.Atoi(raw)
	if err != nil {
		log.Printf("invalid %s=%q, keeping previous value: %v", name, raw, err)
		return fallback
	}

	return time.Duration(seconds) * time.Second
}
