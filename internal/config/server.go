package config

import (
	"flag"
)

// Server хранит настройки сервера.
type Server struct {
	Addr          string // адрес и порт сервера
	StoreInterval int    // период сохранения в файл в секундах, 0 значит писать сразу
	FileStorage   string // файл для хранения метрик
	Restore       bool   // загружать метрики из файла при старте
	DatabaseDSN   string // строка подключения к PostgreSQL
	Key           string // ключ подписи запросов и ответов
	AuditFile     string // файл для событий аудита
	AuditURL      string // адрес приёмника событий аудита
}

// ParseServer читает настройки сервера.
func ParseServer() (Server, error) {
	var cfg Server

	flag.StringVar(&cfg.Addr, "a", "localhost:8080", "адрес и порт запуска сервера")
	flag.IntVar(&cfg.StoreInterval, "i", 300, "интервал сохранения метрик в секундах, при значении 0 писать сразу")
	flag.StringVar(&cfg.FileStorage, "f", "/tmp/metrics-db.json", "файл для хранения метрик")
	flag.BoolVar(&cfg.Restore, "r", true, "загружать сохранённые метрики при старте")
	flag.StringVar(&cfg.DatabaseDSN, "d", "", "строка подключения к базе данных")
	flag.StringVar(&cfg.Key, "k", "", "ключ подписи передаваемых данных")
	flag.StringVar(&cfg.AuditFile, "audit-file", "", "файл для событий аудита, без него аудит в файл выключен")
	flag.StringVar(&cfg.AuditURL, "audit-url", "", "адрес приёмника событий аудита, без него отправка выключена")
	flag.Parse()

	envString("ADDRESS", &cfg.Addr)
	envString("FILE_STORAGE_PATH", &cfg.FileStorage)
	envString("DATABASE_DSN", &cfg.DatabaseDSN)
	envString("KEY", &cfg.Key)
	envString("AUDIT_FILE", &cfg.AuditFile)
	envString("AUDIT_URL", &cfg.AuditURL)

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
