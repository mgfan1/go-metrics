// Package pool хранит переиспользуемые объекты одного типа и сбрасывает их перед возвратом в пул.
package pool

import "sync"

// Resetter умеет сбрасывать своё состояние перед повторным использованием.
type Resetter interface {
	// Reset возвращает объект к начальному состоянию.
	Reset()
}

// Pool хранит объекты типа T и безопасен для конкурентного вызова.
type Pool[T Resetter] struct {
	items sync.Pool
}

// New создаёт пул, newFn вызывается, когда в пуле нет свободного объекта.
func New[T Resetter](newFn func() T) *Pool[T] {
	p := &Pool[T]{}
	p.items.New = func() any { return newFn() }
	return p
}

// Get возвращает объект из пула или новый от newFn.
func (p *Pool[T]) Get() T {
	return p.items.Get().(T)
}

// Put сбрасывает x методом Reset и кладёт его в пул.
func (p *Pool[T]) Put(x T) {
	x.Reset()
	p.items.Put(x)
}
