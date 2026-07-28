package main

import (
	"log"
	"time"

	"github.com/mgfan1/go-metrics/internal/agent"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := parseFlags()
	if err != nil {
		return err
	}

	poll := time.Duration(cfg.pollInterval) * time.Second
	report := time.Duration(cfg.reportInterval) * time.Second

	log.Printf("агент: сервер %s, опрос %s, отправка %s", cfg.addr, poll, report)

	agent.New(cfg.addr, poll, report).Run()
	return nil
}
