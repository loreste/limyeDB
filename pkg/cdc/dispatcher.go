package cdc

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/limyedb/limyedb/pkg/point"
)

// EventType categorizes exactly what modification happened
type EventType string

const (
	EventInsert EventType = "insert"
	EventDelete EventType = "delete"
	EventUpdate EventType = "update"
)

// Event packages the absolute point states and payloads dynamically.
type Event struct {
	Collection string      `json:"collection"`
	Type       EventType   `json:"type"`
	Point      *point.Point `json:"point,omitempty"`
	PointID    string      `json:"point_id,omitempty"`
	Timestamp  int64       `json:"timestamp"`
}

// WebhookSubscription binds an external callback consumer URL efficiently
type WebhookSubscription struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

// Dispatcher globally abstracts channel writes propagating asynchronously.
type Dispatcher struct {
	subscriptions map[string][]WebhookSubscription // collection -> webhooks
	eventCh       chan Event
	client        *http.Client
	mu            sync.RWMutex
}

var globalDispatcher *Dispatcher
var once sync.Once

// GetDispatcher resolves a singleton globally ensuring 1 single routine bounds the worker.
func GetDispatcher() *Dispatcher {
	once.Do(func() {
		globalDispatcher = &Dispatcher{
			subscriptions: make(map[string][]WebhookSubscription),
			eventCh:       make(chan Event, 50000), // Highly elevated buffer handling massive bulk inserts concurrently
			client: &http.Client{
				Timeout: 3 * time.Second, // Fast explicit timeouts avoiding network drops locking processes
			},
		}
		go globalDispatcher.worker()
	})
	return globalDispatcher
}

// Subscribe explicitly binds new users requesting live streaming hooks natively
func (d *Dispatcher) Subscribe(collection string, sub WebhookSubscription) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.subscriptions[collection] = append(d.subscriptions[collection], sub)
}

// Subscriptions returns the array natively
func (d *Dispatcher) Subscriptions(collection string) []WebhookSubscription {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.subscriptions[collection]
}

// Publish streams the mutation safely failing fast if buffer is exhausted dynamically 
func (d *Dispatcher) Publish(event Event) {
	select {
	case d.eventCh <- event:
	default:
		log.Printf("CDC Buffer completely exhausted. Dropping telemetry for %s event on %s", event.Type, event.Collection)
	}
}

// worker constantly dequeues executing explicitly natively mapping IO hooks
func (d *Dispatcher) worker() {
	for event := range d.eventCh {
		d.mu.RLock()
		hooks := d.subscriptions[event.Collection]
		d.mu.RUnlock()

		if len(hooks) == 0 {
			continue
		}

		payload, err := json.Marshal(event)
		if err != nil {
			continue
		}

		for _, hook := range hooks {
			h := hook // Capture correctly explicitly across channels natively
			go func(sub WebhookSubscription, body []byte) {
				req, _ := http.NewRequest("POST", sub.URL, bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				for k, v := range sub.Headers {
					req.Header.Set(k, v)
				}
				resp, err := d.client.Do(req)
				if err == nil {
					resp.Body.Close()
				}
			}(h, payload)
		}
	}
}
