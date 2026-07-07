package main

import "flag"

type config struct {
	addr           string
	reportInterval int
	pollInterval   int
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.addr, "a", "localhost:8080", "адрес сервера метрик")
	flag.IntVar(&cfg.reportInterval, "r", 10, "период отправки метрик, сек")
	flag.IntVar(&cfg.pollInterval, "p", 2, "период опроса метрик, сек")
	flag.Parse()
	return cfg
}
