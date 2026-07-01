package broker

import (
	"fmt"
	"sync"

	"github.com/infinage/microfix/pkg/session"
)

type Producer interface {
	SubscribeLog() (<-chan session.Log, func(), error)
}

type Broker struct {
	subs   map[chan session.Log]any
	cancel func()
	mu     sync.RWMutex
}

func NewBroker() *Broker {
	return &Broker{subs: make(map[chan session.Log]any)}
}

// Channel returned is never closed by broker. DO NOT loop
// through returned channel without a way to unsubscribe
// outside of goroutine that would read on it.
func (b *Broker) Subscribe() (<-chan session.Log, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan session.Log, 256)
	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
	}

	b.subs[ch] = nil
	return ch, unsubscribe
}

// Bind to a new session, existing callers will continue
// to recv updates from new session.
func (b *Broker) Bind(producer Producer) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Force unsub from previous session if still running
	if b.cancel != nil {
		b.cancel()
	}

	ch, unsub, err := producer.SubscribeLog()
	if err != nil {
		return fmt.Errorf("broker log subscription failed: %w", err)
	}

	// Spawn a goroutine to listen on new session
	b.cancel = unsub
	go func() {
		defer unsub()
		for log := range ch {
			b.publish(log)
		}
	}()

	return nil
}

// Send message to all subscribers
func (b *Broker) publish(log session.Log) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- log:
		default: // Drop if channel full
		}
	}
}
