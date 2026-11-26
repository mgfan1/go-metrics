package lib

import (
	"errors"
	"log"
	"os"
)

func mustOpen(name string) *os.File {
	f, err := os.Open(name)
	if err != nil {
		panic(err) // want "вызов встроенной функции panic"
	}
	return f
}

func stop() {
	log.Fatal("стоп") // want `вызов log\.Fatal вне функции main пакета main`
}

func stopf(code int) {
	log.Fatalf("код %d", code) // want `вызов log\.Fatalf вне функции main пакета main`
}

func exitLater() {
	defer os.Exit(1) // want `вызов os\.Exit вне функции main пакета main`
}

func main() {
	os.Exit(0) // want `вызов os\.Exit вне функции main пакета main`
}

var onFatal = func() {
	os.Exit(2) // want `вызов os\.Exit вне функции main пакета main`
}

func ownPanic() {
	panic := func(err error) { _ = err }
	panic(errors.New("это не встроенная функция"))
}

func parenPanic() {
	(panic)("в скобках") // want "вызов встроенной функции panic"
}
