package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

var _ Observer = (*FileObserver)(nil)

type FileObserver struct {
	file *os.File
}

func NewFileObserver(path string) (*FileObserver, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("файл аудита: %w", err)
	}

	return &FileObserver{file: f}, nil
}

func (o *FileObserver) Notify(_ context.Context, e Event) error {
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}

	_, err = o.file.Write(append(line, '\n'))
	return err
}

func (o *FileObserver) Close() error {
	return o.file.Close()
}
