package lib

import (
	. "log"
	. "os"
)

func quitDot() {
	Exit(1) // want `вызов os\.Exit вне функции main пакета main`
}

func stopDot() {
	Fatal("стоп") // want `вызов log\.Fatal вне функции main пакета main`
}
