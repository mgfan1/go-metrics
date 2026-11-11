package config

import (
	"flag"
	"fmt"
)

// Agent хранит настройки агента.
type Agent struct {
	Addr           string // адрес сервера метрик
	ReportInterval int    // период отправки в секундах
	PollInterval   int    // период опроса в секундах
	Key            string // ключ подписи отправляемых данных
	RateLimit      int    // предел одновременных запросов к серверу
}

// ParseAgent читает настройки агента и проверяет, что периоды больше нуля.
func ParseAgent() (Agent, error) {
	var cfg Agent

	flag.StringVar(&cfg.Addr, "a", "localhost:8080", "адрес сервера метрик")
	flag.IntVar(&cfg.ReportInterval, "r", 10, "период отправки метрик, сек")
	flag.IntVar(&cfg.PollInterval, "p", 2, "период опроса метрик, сек")
	flag.StringVar(&cfg.Key, "k", "", "ключ подписи передаваемых данных")
	flag.IntVar(&cfg.RateLimit, "l", 1, "число одновременных запросов к серверу")
	flag.Parse()

	envString("ADDRESS", &cfg.Addr)
	envString("KEY", &cfg.Key)

	for _, err := range []error{
		envInt("REPORT_INTERVAL", &cfg.ReportInterval),
		envInt("POLL_INTERVAL", &cfg.PollInterval),
		envInt("RATE_LIMIT", &cfg.RateLimit),
	} {
		if err != nil {
			return cfg, err
		}
	}

	if cfg.RateLimit <= 0 {
		cfg.RateLimit = 1
	}

	if cfg.ReportInterval <= 0 {
		return cfg, fmt.Errorf("период отправки должен быть больше нуля, получено %d", cfg.ReportInterval)
	}
	if cfg.PollInterval <= 0 {
		return cfg, fmt.Errorf("период опроса должен быть больше нуля, получено %d", cfg.PollInterval)
	}

	return cfg, nil
}
