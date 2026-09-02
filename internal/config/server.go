package config

import (
	"flag"
)

type Server struct {
	Addr          string
	StoreInterval int
	FileStorage   string
	Restore       bool
	DatabaseDSN   string
	Key           string
}

func ParseServer() (Server, error) {
	var cfg Server

	flag.StringVar(&cfg.Addr, "a", "localhost:8080", "адрес и порт запуска сервера")
	flag.IntVar(&cfg.StoreInterval, "i", 300, "интервал сохранения метрик в секундах, при значении 0 писать сразу")
	flag.StringVar(&cfg.FileStorage, "f", "/tmp/metrics-db.json", "файл для хранения метрик")
	flag.BoolVar(&cfg.Restore, "r", true, "загружать сохранённые метрики при старте")
	flag.StringVar(&cfg.DatabaseDSN, "d", "", "строка подключения к базе данных")
	flag.StringVar(&cfg.Key, "k", "", "ключ подписи передаваемых данных")
	flag.Parse()

	envString("ADDRESS", &cfg.Addr)
	envString("FILE_STORAGE_PATH", &cfg.FileStorage)
	envString("DATABASE_DSN", &cfg.DatabaseDSN)
	envString("KEY", &cfg.Key)

	for _, err := range []error{
		envInt("STORE_INTERVAL", &cfg.StoreInterval),
		envBool("RESTORE", &cfg.Restore),
	} {
		if err != nil {
			return cfg, err
		}
	}

	return cfg, nil
}
