package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
)

type config struct {
	addr          string
	storeInterval int
	fileStorage   string
	restore       bool
}

func parseFlags() (config, error) {
	var cfg config

	flag.StringVar(&cfg.addr, "a", "localhost:8080", "адрес и порт запуска сервера")
	flag.IntVar(&cfg.storeInterval, "i", 300, "интервал сохранения метрик в секундах, при значении 0 писать сразу")
	flag.StringVar(&cfg.fileStorage, "f", "/tmp/metrics-db.json", "файл для хранения метрик")
	flag.BoolVar(&cfg.restore, "r", true, "загружать сохранённые метрики при старте")
	flag.Parse()

	if v := os.Getenv("ADDRESS"); v != "" {
		cfg.addr = v
	}
	if v := os.Getenv("STORE_INTERVAL"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("STORE_INTERVAL: %w", err)
		}
		cfg.storeInterval = n
	}
	if v, ok := os.LookupEnv("FILE_STORAGE_PATH"); ok {
		cfg.fileStorage = v
	}
	if v := os.Getenv("RESTORE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return cfg, fmt.Errorf("RESTORE: %w", err)
		}
		cfg.restore = b
	}

	return cfg, nil
}
