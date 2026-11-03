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
	if cfg.addr != "localhost:8080" || cfg.reportInterval != 10*time.Second ||
		cfg.pollInterval != 2*time.Second || cfg.key != "" || cfg.cryptoKey != "" ||
		cfg.rateLimit != 1 {
		t.Errorf("дефолты изменились: %+v", cfg)
	}
}

func TestLoadConfig_FileOnly(t *testing.T) {
	path := writeConfig(t, `{
		"address": "10.0.0.1:9000",
		"report_interval": "1s",
		"poll_interval": "300ms",
		"key": "file-key",
		"crypto_key": "/tmp/file.pem",
		"rate_limit": 5
	}`)

	cfg, err := loadConfig([]string{"-c", path}, envFrom(nil))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	if cfg.addr != "10.0.0.1:9000" || cfg.reportInterval != time.Second ||
		cfg.pollInterval != 300*time.Millisecond || cfg.key != "file-key" ||
		cfg.cryptoKey != "/tmp/file.pem" || cfg.rateLimit != 5 {
		t.Errorf("значения из файла не применились: %+v", cfg)
	}
}

func TestLoadConfig_Priority(t *testing.T) {
	fileJSON := `{"address": "10.0.0.1:9000", "poll_interval": "5s"}`

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
			if cfg.pollInterval != 5*time.Second {
				t.Errorf("pollInterval = %v, want 5s (из файла)", cfg.pollInterval)
			}
		})
	}
}

// rate_limit: 0 из файла - валидное присутствующее значение; итог всё равно
// приводится к минимуму 1, как и раньше для флага/env.
func TestLoadConfig_RateLimitClamped(t *testing.T) {
	path := writeConfig(t, `{"rate_limit": 0}`)

	cfg, err := loadConfig([]string{"-c", path}, envFrom(nil))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.rateLimit != 1 {
		t.Errorf("rateLimit = %d, want 1 (clamp)", cfg.rateLimit)
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
		path    string
	}{
		{"несуществующий файл", "", filepath.Join(t.TempDir(), "missing.json")},
		{"битый JSON", "{невалидно", ""},
		{"неверная длительность", `{"poll_interval": "abc"}`, ""},
		{"отрицательная длительность", `{"report_interval": "-1s"}`, ""},
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
