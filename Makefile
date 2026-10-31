# Значения для -ldflags. Можно переопределить из окружения:
#   make build BUILD_VERSION=v1.2.3
BUILD_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null)
BUILD_DATE    ?= $(shell date -u +%Y/%m/%d\ %H:%M:%S)
BUILD_COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null)

# Подстановка информации о сборке в переменные package main.
LDFLAGS = -X 'main.buildVersion=$(BUILD_VERSION)' \
          -X 'main.buildDate=$(BUILD_DATE)' \
          -X 'main.buildCommit=$(BUILD_COMMIT)'

.PHONY: build build-server build-agent clean

# Сборка обеих команд.
build: build-server build-agent

# Бинарник сервера рядом с его пакетом (так его ищут автотесты Практикума).
build-server:
	go build -ldflags "$(LDFLAGS)" -o cmd/server/server ./cmd/server

# Бинарник агента рядом с его пакетом.
build-agent:
	go build -ldflags "$(LDFLAGS)" -o cmd/agent/agent ./cmd/agent

# Удаление собранных бинарников.
clean:
	rm -f cmd/server/server cmd/agent/agent
