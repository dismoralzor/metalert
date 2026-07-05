package main

import (
	"flag"
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

	return config{
		addr:           *addr,
		pollInterval:   time.Duration(*pollInterval) * time.Second,
		reportInterval: time.Duration(*reportInterval) * time.Second,
	}
}
