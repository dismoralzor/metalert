package main

import (
	"flag"
	"log"
	"strconv"
	"time"

	filecfg "github.com/dismoralzor/metalert/internal/config"
)

type config struct {
	addr     string
	logLevel string
	// storeInterval == 0 означает синхронную запись после каждого обновления.
	storeInterval   time.Duration
	fileStoragePath string
	restore         bool
	dsn             string
	key             string
	// cryptoKey - путь к файлу с приватным RSA-ключом. Пусто = шифрование выключено.
	cryptoKey string
	// Аудит запросов (паттерн Наблюдатель). Пустая строка = приёмник выключен.
	auditFile string
	auditURL  string
}

// loadConfig собирает конфигурацию слоями по нарастанию приоритета:
// дефолты -> JSON-файл (путь из -c/-config или CONFIG) -> флаги (только
// реально переданные) -> переменные окружения (только реально заданные).
// lookupEnv вынесен в параметр ради тестов, в проде это os.LookupEnv.
func loadConfig(args []string, lookupEnv func(string) (string, bool)) (config, error) {
	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	addr := fs.String("a", "localhost:8080", "адрес HTTP-сервера")
	level := fs.String("l", "info", "уровень логирования")
	storeInterval := fs.Int("i", 300, "интервал сохранения в файл, сек (0 - синхронно)")
	fileStoragePath := fs.String("f", "metrics.json", "путь к файлу с метриками")
	restoreFlag := fs.Bool("r", true, "загружать метрики из файла при старте")
	dsn := fs.String("d", "", "строка подключения к БД (DSN)")
	key := fs.String("k", "", "ключ для подписи запросов/ответов HashSHA256 (пусто - подпись выключена)")
	cryptoKey := fs.String("crypto-key", "", "путь к файлу с приватным RSA-ключом (пусто - шифрование выключено)")
	// fs.String("audit-file", ...) принимает и -audit-file, и --audit-file.
	auditFile := fs.String("audit-file", "", "путь к файлу аудита запросов (пусто - выключено)")
	auditURL := fs.String("audit-url", "", "URL приёмника аудита запросов (пусто - выключено)")
	// -c и -config - один параметр под двумя именами.
	cfgPath := fs.String("c", "", "путь к JSON-файлу конфигурации")
	fs.StringVar(cfgPath, "config", "", "путь к JSON-файлу конфигурации")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	// Слой дефолтов (в указателях флагов после Parse - либо дефолт, либо
	// переданное значение; переданные переустановим ниже через Visit).
	cfg := config{
		addr:            *addr,
		logLevel:        *level,
		storeInterval:   time.Duration(*storeInterval) * time.Second,
		fileStoragePath: *fileStoragePath,
		restore:         *restoreFlag,
		dsn:             *dsn,
		key:             *key,
		cryptoKey:       *cryptoKey,
		auditFile:       *auditFile,
		auditURL:        *auditURL,
	}

	// Путь к конфигу: env CONFIG перекрывает флаг, как у остальных опций.
	path := *cfgPath
	if envPath, ok := lookupEnv("CONFIG"); ok && envPath != "" {
		path = envPath
	}

	// Слой файла: применяем только поля, реально присутствующие в JSON.
	if path != "" {
		var fc filecfg.ServerFile
		if err := filecfg.LoadJSON(path, &fc); err != nil {
			return config{}, err
		}
		if fc.Address != nil {
			cfg.addr = *fc.Address
		}
		if fc.LogLevel != nil {
			cfg.logLevel = *fc.LogLevel
		}
		if fc.StoreInterval != nil {
			cfg.storeInterval = fc.StoreInterval.Duration
		}
		if fc.StoreFile != nil {
			cfg.fileStoragePath = *fc.StoreFile
		}
		if fc.Restore != nil {
			cfg.restore = *fc.Restore
		}
		if fc.DatabaseDSN != nil {
			cfg.dsn = *fc.DatabaseDSN
		}
		if fc.Key != nil {
			cfg.key = *fc.Key
		}
		if fc.CryptoKey != nil {
			cfg.cryptoKey = *fc.CryptoKey
		}
		if fc.AuditFile != nil {
			cfg.auditFile = *fc.AuditFile
		}
		if fc.AuditURL != nil {
			cfg.auditURL = *fc.AuditURL
		}
	}

	// Слой флагов: применяем только реально переданные - сравнение с дефолтом
	// не отличило бы "-a localhost:8080" от "флаг не задан".
	visited := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	if visited["a"] {
		cfg.addr = *addr
	}
	if visited["l"] {
		cfg.logLevel = *level
	}
	if visited["i"] {
		cfg.storeInterval = time.Duration(*storeInterval) * time.Second
	}
	if visited["f"] {
		cfg.fileStoragePath = *fileStoragePath
	}
	if visited["r"] {
		cfg.restore = *restoreFlag
	}
	if visited["d"] {
		cfg.dsn = *dsn
	}
	if visited["k"] {
		cfg.key = *key
	}
	if visited["crypto-key"] {
		cfg.cryptoKey = *cryptoKey
	}
	if visited["audit-file"] {
		cfg.auditFile = *auditFile
	}
	if visited["audit-url"] {
		cfg.auditURL = *auditURL
	}

	// Слой env: реально заданная переменная перекрывает всё остальное.
	// Пустая строка значения не перетирает, кривые числа/булевы - ворнинг
	// в лог и сохранение предыдущего значения (прежнее поведение).
	if v, ok := lookupEnv("ADDRESS"); ok && v != "" {
		cfg.addr = v
	}
	if v, ok := lookupEnv("LOG_LEVEL"); ok && v != "" {
		cfg.logLevel = v
	}
	if v, ok := lookupEnv("FILE_STORAGE_PATH"); ok && v != "" {
		cfg.fileStoragePath = v
	}
	if raw, ok := lookupEnv("STORE_INTERVAL"); ok && raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil {
			log.Printf("invalid STORE_INTERVAL=%q, keeping previous value: %v", raw, err)
		} else {
			cfg.storeInterval = time.Duration(seconds) * time.Second
		}
	}
	if raw, ok := lookupEnv("RESTORE"); ok && raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			log.Printf("invalid RESTORE=%q, keeping previous value: %v", raw, err)
		} else {
			cfg.restore = parsed
		}
	}
	if v, ok := lookupEnv("DATABASE_DSN"); ok && v != "" {
		cfg.dsn = v
	}
	if v, ok := lookupEnv("KEY"); ok && v != "" {
		cfg.key = v
	}
	if v, ok := lookupEnv("CRYPTO_KEY"); ok && v != "" {
		cfg.cryptoKey = v
	}
	if v, ok := lookupEnv("AUDIT_FILE"); ok && v != "" {
		cfg.auditFile = v
	}
	if v, ok := lookupEnv("AUDIT_URL"); ok && v != "" {
		cfg.auditURL = v
	}

	return cfg, nil
}
