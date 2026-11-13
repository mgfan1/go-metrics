package audit

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	queueSize    = 256
	closeTimeout = 5 * time.Second
)

// Event описывает одно событие аудита полученных метрик.
type Event struct {
	TS        int64    `json:"ts"`         // время события в секундах Unix
	Metrics   []string `json:"metrics"`    // имена полученных метрик
	IPAddress string   `json:"ip_address"` // IP-адрес клиента
}

// Observer получает события аудита.
type Observer interface {
	// Notify доставляет событие наблюдателю.
	Notify(ctx context.Context, e Event) error
}

type subscriber struct {
	observer Observer
	events   chan Event
	done     chan struct{}
}

// Publisher рассылает события зарегистрированным наблюдателям, каждому в своей горутине.
type Publisher struct {
	mu          sync.RWMutex
	subscribers []*subscriber
	closed      bool
	ctx         context.Context
	cancel      context.CancelFunc
	log         *zap.Logger
}

// NewPublisher создаёт издателя без наблюдателей.
func NewPublisher(log *zap.Logger) *Publisher {
	ctx, cancel := context.WithCancel(context.Background())
	return &Publisher{ctx: ctx, cancel: cancel, log: log}
}

// Register подписывает наблюдателя и запускает доставку ему событий.
func (p *Publisher) Register(o Observer) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	s := &subscriber{observer: o, events: make(chan Event, queueSize), done: make(chan struct{})}
	p.subscribers = append(p.subscribers, s)
	go p.deliver(s)
}

// Publish ставит событие в очереди наблюдателей и не ждёт доставки, после Close и при переполненной очереди событие теряется.
func (p *Publisher) Publish(e Event) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return
	}

	for _, s := range p.subscribers {
		select {
		case s.events <- e:
		default:
			p.log.Warn("очередь аудита переполнена, событие пропущено", zap.Strings("metrics", e.Metrics))
		}
	}
}

// Close дорассылает очереди с ограничением по времени и закрывает наблюдателей, повторный вызов ничего не делает.
func (p *Publisher) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	for _, s := range p.subscribers {
		close(s.events)
	}
	p.mu.Unlock()

	timer := time.AfterFunc(closeTimeout, p.cancel)
	defer timer.Stop()
	defer p.cancel()

	var errs []error
	for _, s := range p.subscribers {
		<-s.done
		if c, ok := s.observer.(io.Closer); ok {
			errs = append(errs, c.Close())
		}
	}

	return errors.Join(errs...)
}

func (p *Publisher) deliver(s *subscriber) {
	defer close(s.done)

	skipped := 0
	for e := range s.events {
		if p.ctx.Err() != nil {
			skipped++
			continue
		}
		if err := s.observer.Notify(p.ctx, e); err != nil {
			p.log.Warn("не отправил событие аудита", zap.Error(err))
		}
	}

	if skipped > 0 {
		p.log.Warn("аудит остановлен по таймауту, часть событий не отправлена", zap.Int("skipped", skipped))
	}
}
