package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// ServerFile - конфигурация сервера из JSON-файла. Все поля - указатели:
// nil означает "поля в файле нет", такое поле не применяется и оставляет
// нетронутыми значения из флагов, env и дефолтов.
type ServerFile struct {
	Address       *string   `json:"address"`
	LogLevel      *string   `json:"log_level"`
	StoreInterval *Duration `json:"store_interval"`
	StoreFile     *string   `json:"store_file"`
	Restore       *bool     `json:"restore"`
	DatabaseDSN   *string   `json:"database_dsn"`
	Key           *string   `json:"key"`
	CryptoKey     *string   `json:"crypto_key"`
	AuditFile     *string   `json:"audit_file"`
	AuditURL      *string   `json:"audit_url"`
}

// AgentFile - конфигурация агента из JSON-файла. Семантика полей-указателей
// та же, что у ServerFile.
type AgentFile struct {
	Address        *string   `json:"address"`
	ReportInterval *Duration `json:"report_interval"`
	PollInterval   *Duration `json:"poll_interval"`
	Key            *string   `json:"key"`
	CryptoKey      *string   `json:"crypto_key"`
	RateLimit      *int      `json:"rate_limit"`
}

// LoadJSON читает JSON-файл конфигурации path в структуру v. Неизвестные
// ключи игнорируются: один и тот же файл не должен ломаться о лишние поля.
func LoadJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file %s: %w", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse config file %s: %w", path, err)
	}
	return nil
}
