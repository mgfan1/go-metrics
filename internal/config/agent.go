package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
)

type Agent struct {
	Addr           string
	ReportInterval int
	PollInterval   int
	Key            string
}

func ParseAgent() (Agent, error) {
	var cfg Agent

	flag.StringVar(&cfg.Addr, "a", "localhost:8080", "адрес сервера метрик")
	flag.IntVar(&cfg.ReportInterval, "r", 10, "период отправки метрик, сек")
	flag.IntVar(&cfg.PollInterval, "p", 2, "период опроса метрик, сек")
	flag.StringVar(&cfg.Key, "k", "", "ключ подписи передаваемых данных")
	flag.Parse()

	if v, ok := os.LookupEnv("ADDRESS"); ok {
		cfg.Addr = v
	}
	if v, ok := os.LookupEnv("REPORT_INTERVAL"); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("REPORT_INTERVAL: %w", err)
		}
		cfg.ReportInterval = n
	}
	if v, ok := os.LookupEnv("POLL_INTERVAL"); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("POLL_INTERVAL: %w", err)
		}
		cfg.PollInterval = n
	}
	if v, ok := os.LookupEnv("KEY"); ok {
		cfg.Key = v
	}

	if cfg.ReportInterval <= 0 {
		return cfg, fmt.Errorf("период отправки должен быть больше нуля, получено %d", cfg.ReportInterval)
	}
	if cfg.PollInterval <= 0 {
		return cfg, fmt.Errorf("период опроса должен быть больше нуля, получено %d", cfg.PollInterval)
	}

	return cfg, nil
}
