package broker

import (
	"sync"
	"testing"
	"time"

	"github.com/infinage/microfix/pkg/session"
)

// mockProducer satisfies the Producer interface
type mockProducer struct {
	ch       chan session.Log
	isClosed bool
	mu       sync.Mutex
}

func newMockProducer() *mockProducer {
	return &mockProducer{
		ch: make(chan session.Log, 10),
	}
}

func (m *mockProducer) SubscribeLog() (<-chan session.Log, func(), error) {
	unsub := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if !m.isClosed {
			m.isClosed = true
			close(m.ch)
		}
	}
	return m.ch, unsub, nil
}

func (m *mockProducer) Push(log session.Log) {
	m.ch <- log
}

func TestBroker_PubSub(t *testing.T) {
	b := NewBroker()
	producer := newMockProducer()
	b.Bind(producer)

	// Create two subscribers
	sub1, unsub1 := b.Subscribe()
	sub2, unsub2 := b.Subscribe()

	// Push a log
	testLog := session.Log{} // Dummy log
	producer.Push(testLog)

	// Verify both received it
	select {
	case <-sub1:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("sub1 missed log")
	}

	select {
	case <-sub2:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("sub2 missed log")
	}

	// Unsubscribe 1, push again
	unsub1()
	producer.Push(testLog)

	// Sub 2 should get it, sub 1 should be closed
	select {
	case <-sub2:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("sub2 missed second log")
	}

	select {
	case _, ok := <-sub1:
		if ok {
			t.Fatal("sub1 channel should be closed")
		}
	default:
		// Closed channels return immediately, if it hits default it wasn't closed
	}

	unsub2()
}

func TestBroker_BindReplacesOldProducer(t *testing.T) {
	b := NewBroker()

	prod1 := newMockProducer()
	prod2 := newMockProducer()

	b.Bind(prod1)
	b.Bind(prod2)

	// Wait a tiny bit for the goroutine scheduling
	time.Sleep(10 * time.Millisecond)

	prod1.mu.Lock()
	closed := prod1.isClosed
	prod1.mu.Unlock()

	if !closed {
		t.Fatal("prod1 was not forcefully unsubscribed when prod2 was bound")
	}
}

func TestBroker_NonBlockingSlowClient(t *testing.T) {
	b := NewBroker()
	producer := newMockProducer()
	b.Bind(producer)

	// Make a subscriber but never read from it
	_, unsub := b.Subscribe()
	defer unsub()

	// Spam the producer way past the subscriber's 256 channel capacity
	for range 300 {
		producer.Push(session.Log{})
	}

	// Give the background goroutine a moment to process
	time.Sleep(50 * time.Millisecond)
}
