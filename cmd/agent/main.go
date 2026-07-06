package main

import (
	"log"

	"github.com/mgfan1/go-metrics/internal/agent"
)

func main() {
	cfg := parseFlags()

	log.Printf("агент: сервер %s, опрос %s, отправка %s",
		cfg.addr, cfg.pollInterval, cfg.reportInterval)

	agent.New(cfg.addr, cfg.pollInterval, cfg.reportInterval).Run()
}
