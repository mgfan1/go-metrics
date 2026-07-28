package main

import (
	"flag"
	"os"
)

type config struct {
	addr string
}

func parseFlags() config {
	var cfg config

	flag.StringVar(&cfg.addr, "a", "localhost:8080", "адрес и порт запуска сервера")
	flag.Parse()

	if v := os.Getenv("ADDRESS"); v != "" {
		cfg.addr = v
	}

	return cfg
}
