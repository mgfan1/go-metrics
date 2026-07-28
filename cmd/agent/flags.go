package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
)

type config struct {
	addr           string
	reportInterval int
	pollInterval   int
}

func parseFlags() (config, error) {
	var cfg config

	flag.StringVar(&cfg.addr, "a", "localhost:8080", "адрес сервера метрик")
	flag.IntVar(&cfg.reportInterval, "r", 10, "период отправки метрик, сек")
	flag.IntVar(&cfg.pollInterval, "p", 2, "период опроса метрик, сек")
	flag.Parse()

	if v := os.Getenv("ADDRESS"); v != "" {
		cfg.addr = v
	}
	if v := os.Getenv("REPORT_INTERVAL"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("REPORT_INTERVAL: %w", err)
		}
		cfg.reportInterval = n
	}
	if v := os.Getenv("POLL_INTERVAL"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("POLL_INTERVAL: %w", err)
		}
		cfg.pollInterval = n
	}

	if cfg.reportInterval <= 0 {
		return cfg, fmt.Errorf("период отправки должен быть больше нуля, получено %d", cfg.reportInterval)
	}
	if cfg.pollInterval <= 0 {
		return cfg, fmt.Errorf("период опроса должен быть больше нуля, получено %d", cfg.pollInterval)
	}

	return cfg, nil
}
