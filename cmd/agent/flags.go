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
	addr := flag.String("a", "localhost:8080", "адрес сервера метрик")
	report := flag.Int("r", 10, "период отправки метрик, сек")
	poll := flag.Int("p", 2, "период опроса метрик, сек")
	flag.Parse()

	return config{
		addr:           *addr,
		pollInterval:   time.Duration(*poll) * time.Second,
		reportInterval: time.Duration(*report) * time.Second,
	}
}
