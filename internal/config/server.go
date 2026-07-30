package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
)

type Server struct {
	Addr          string
	StoreInterval int
	FileStorage   string
	Restore       bool
}

func ParseServer() (Server, error) {
	var cfg Server

	flag.StringVar(&cfg.Addr, "a", "localhost:8080", "адрес и порт запуска сервера")
	flag.IntVar(&cfg.StoreInterval, "i", 300, "интервал сохранения метрик в секундах, при значении 0 писать сразу")
	flag.StringVar(&cfg.FileStorage, "f", "/tmp/metrics-db.json", "файл для хранения метрик")
	flag.BoolVar(&cfg.Restore, "r", true, "загружать сохранённые метрики при старте")
	flag.Parse()

	if v, ok := os.LookupEnv("ADDRESS"); ok {
		cfg.Addr = v
	}
	if v, ok := os.LookupEnv("STORE_INTERVAL"); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("STORE_INTERVAL: %w", err)
		}
		cfg.StoreInterval = n
	}
	if v, ok := os.LookupEnv("FILE_STORAGE_PATH"); ok {
		cfg.FileStorage = v
	}
	if v, ok := os.LookupEnv("RESTORE"); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return cfg, fmt.Errorf("RESTORE: %w", err)
		}
		cfg.Restore = b
	}

	return cfg, nil
}
