package main

import "flag"

type config struct {
	addr string
}

func parseFlags() config {
	addr := flag.String("a", "localhost:8080", "адрес HTTP-сервера")
	flag.Parse()

	return config{addr: *addr}
}
