package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeConfig складывает JSON-конфиг во временный каталог теста.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

// envFrom делает lookupEnv из мапы - тесты изолированы от реального окружения.
func envFrom(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	cfg, err := loadConfig(nil, envFrom(nil))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.addr != "localhost:8080" || cfg.logLevel != "info" ||
		cfg.storeInterval != 300*time.Second || cfg.fileStoragePath != "metrics.json" ||
		!cfg.restore || cfg.dsn != "" || cfg.key != "" || cfg.cryptoKey != "" ||
		cfg.auditFile != "" || cfg.auditURL != "" {
		t.Errorf("дефолты изменились: %+v", cfg)
	}
}

const fullServerJSON = `{
	"address": "10.0.0.1:9000",
	"log_level": "debug",
	"store_interval": "1s",
	"store_file": "/tmp/from-file.db",
	"restore": false,
	"database_dsn": "postgres://file",
	"key": "file-key",
	"crypto_key": "/tmp/file.pem",
	"audit_file": "/tmp/audit.log",
	"audit_url": "http://audit"
}`

func TestLoadConfig_FileOnly(t *testing.T) {
	path := writeConfig(t, fullServerJSON)

	cfg, err := loadConfig([]string{"-c", path}, envFrom(nil))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	if cfg.addr != "10.0.0.1:9000" || cfg.logLevel != "debug" ||
		cfg.storeInterval != time.Second || cfg.fileStoragePath != "/tmp/from-file.db" ||
		cfg.restore || cfg.dsn != "postgres://file" || cfg.key != "file-key" ||
		cfg.cryptoKey != "/tmp/file.pem" || cfg.auditFile != "/tmp/audit.log" ||
		cfg.auditURL != "http://audit" {
		t.Errorf("значения из файла не применились: %+v", cfg)
	}
}

func TestLoadConfig_Priority(t *testing.T) {
	fileJSON := `{"address": "10.0.0.1:9000", "store_interval": "5s"}`

	tests := []struct {
		name     string
		args     func(path string) []string
		env      map[string]string
		wantAddr string
	}{
		{"файл + флаг: побеждает флаг",
			func(p string) []string { return []string{"-c", p, "-a", "flag:1"} },
			nil, "flag:1"},
		{"файл + env: побеждает env",
			func(p string) []string { return []string{"-c", p} },
			map[string]string{"ADDRESS": "env:2"}, "env:2"},
		{"файл + флаг + env: побеждает env",
			func(p string) []string { return []string{"-c", p, "-a", "flag:1"} },
			map[string]string{"ADDRESS": "env:2"}, "env:2"},
		// Флаг со значением, совпадающим с дефолтом, всё равно важнее файла -
		// ловит ошибку "определять переданность флага сравнением с дефолтом".
		{"флаг == дефолту важнее файла",
			func(p string) []string { return []string{"-c", p, "-a", "localhost:8080"} },
			nil, "localhost:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, fileJSON)
			cfg, err := loadConfig(tt.args(path), envFrom(tt.env))
			if err != nil {
				t.Fatalf("loadConfig: %v", err)
			}
			if cfg.addr != tt.wantAddr {
				t.Errorf("addr = %q, want %q", cfg.addr, tt.wantAddr)
			}
			// Неперекрытое поле остаётся из файла.
			if cfg.storeInterval != 5*time.Second {
				t.Errorf("storeInterval = %v, want 5s (из файла)", cfg.storeInterval)
			}
		})
	}
}

// "restore": false и "store_interval": "0s" - валидные значения, а не
// "поле отсутствует": они обязаны примениться поверх дефолтов.
func TestLoadConfig_ZeroValuesFromFile(t *testing.T) {
	path := writeConfig(t, `{"restore": false, "store_interval": "0s"}`)

	cfg, err := loadConfig([]string{"-c", path}, envFrom(nil))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.restore {
		t.Error("restore = true, want false из файла")
	}
	if cfg.storeInterval != 0 {
		t.Errorf("storeInterval = %v, want 0 (синхронная запись)", cfg.storeInterval)
	}
}

// Путь к конфигу принимается через -c, -config и env CONFIG.
func TestLoadConfig_ConfigPathSources(t *testing.T) {
	fileJSON := `{"address": "file:9999"}`

	t.Run("-config", func(t *testing.T) {
		path := writeConfig(t, fileJSON)
		cfg, err := loadConfig([]string{"-config", path}, envFrom(nil))
		if err != nil {
			t.Fatalf("loadConfig: %v", err)
		}
		if cfg.addr != "file:9999" {
			t.Errorf("addr = %q, want file:9999", cfg.addr)
		}
	})

	t.Run("env CONFIG", func(t *testing.T) {
		path := writeConfig(t, fileJSON)
		cfg, err := loadConfig(nil, envFrom(map[string]string{"CONFIG": path}))
		if err != nil {
			t.Fatalf("loadConfig: %v", err)
		}
		if cfg.addr != "file:9999" {
			t.Errorf("addr = %q, want file:9999", cfg.addr)
		}
	})
}

func TestLoadConfig_Errors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		path    string // если задан, перекрывает путь из content
	}{
		{"несуществующий файл", "", filepath.Join(t.TempDir(), "missing.json")},
		{"битый JSON", "{невалидно", ""},
		{"неверная длительность", `{"store_interval": "abc"}`, ""},
		{"отрицательная длительность", `{"store_interval": "-1s"}`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.path
			if path == "" {
				path = writeConfig(t, tt.content)
			}
			if _, err := loadConfig([]string{"-c", path}, envFrom(nil)); err == nil {
				t.Error("ожидалась ошибка загрузки конфига, получен nil")
			}
		})
	}
}

// Лишние ключи в JSON не должны ломать загрузку.
func TestLoadConfig_UnknownKeysIgnored(t *testing.T) {
	path := writeConfig(t, `{"address": "file:1", "unknown_option": 42}`)

	cfg, err := loadConfig([]string{"-c", path}, envFrom(nil))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.addr != "file:1" {
		t.Errorf("addr = %q, want file:1", cfg.addr)
	}
}
