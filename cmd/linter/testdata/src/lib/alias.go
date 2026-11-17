package lib

import logger "log"

func stopAliased() {
	logger.Fatalln("стоп") // want `вызов log\.Fatalln вне функции main пакета main`
}

func stopLogger() {
	logger.Default().Fatal("стоп") // want `вызов \(\*log\.Logger\)\.Fatal вне функции main пакета main`
}

func printOnly() {
	logger.Println("log.Println не завершает программу")
}
