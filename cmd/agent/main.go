package main

import (
	"time"

	"github.com/mgfan1/go-metrics/internal/agent"
)

func main() {
	agent.New("localhost:8080", 2*time.Second, 10*time.Second).Run()
}
