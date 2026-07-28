package logger

import "go.uber.org/zap"

var Log = zap.NewNop()

func Initialize() error {
	zl, err := zap.NewProduction()
	if err != nil {
		return err
	}
	Log = zl
	return nil
}
