// Package models описывает метрики, которыми обмениваются агент и сервер.
package models

// Типы метрик для поля MType.
const (
	Counter = "counter" // значения складываются
	Gauge   = "gauge"   // хранится последнее значение
)

// Metrics описывает метрику в запросах и ответах сервера.
//
// NOTE: Не усложняем пример, вводя иерархическую вложенность структур.
// Органичиваясь плоской моделью.
// Delta и Value объявлены через указатели,
// что бы отличать значение "0", от не заданного значения
// и соответственно не кодировать в структуру.
type Metrics struct {
	ID    string   `json:"id"`              // имя метрики
	MType string   `json:"type"`            // тип: gauge или counter
	Delta *int64   `json:"delta,omitempty"` // значение counter
	Value *float64 `json:"value,omitempty"` // значение gauge
	Hash  string   `json:"hash,omitempty"`  // не используется, подпись передаётся в заголовке HashSHA256
}
