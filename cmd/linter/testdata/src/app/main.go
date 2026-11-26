package main

import (
	"errors"
	"log"
	"os"
)

func main() {
	if len(os.Args) > 2 {
		log.Fatal("лишние аргументы")
	}

	exit := func(code int) {
		os.Exit(code)
	}

	if err := run(); err != nil {
		exit(1)
	}

	panic("сюда не дойдём") // want "вызов встроенной функции panic"
}

func run() error {
	return errors.New("ошибка")
}

func init() {
	if os.Getenv("APP_DISABLED") != "" {
		os.Exit(3) // want `вызов os\.Exit вне функции main пакета main`
	}
}

type service struct{}

func (service) main() {
	os.Exit(4) // want `вызов os\.Exit вне функции main пакета main`
}
