// Package webhook provides webhook notification support for LimyeDB.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// EventType represents the type of webhook event.
type EventType string

const (
	EventPointInsert     EventType = "point.insert"
	EventPointUpdate     EventType = "point.update"
	EventPointDelete     EventType = "point.delete"
	EventCollectionCreate EventType = "collection.create"
	EventCollectionDelete EventType = "collection.delete"
	EventSnapshotCreate  EventType = "snapshot.create"
	EventClusterJoin     EventType = "cluster.join"
	EventClusterLeave    EventType = "cluster.leave"
)

// Event represents a webhook event.
type Event struct {
	ID        string                 `json:"id"`
	Type      EventType              `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// Subscription represents a webhook subscription.
type Subscription struct {
	ID         string      `json:"id"`
	URL        string      `json:"url"`
	Events     []EventType `json:"events"`
	Secret     string      `json:"secret,omitempty"`
	Active     bool        `json:"active"`
	CreatedAt  time.Time   `json:"created_at"`
	Collection string      `json:"collection,omitempty"` // Optional: filter by collection
	Headers    map[string]string `json:"headers,omitempty"`    // Custom headers
}

// DeliveryResult represents the result of a webhook delivery.
type DeliveryResult struct {
	SubscriptionID string
	EventID        string
	StatusCode     int
	Error          error
	Duration       time.Duration
	Timestamp      time.Time
}

// Manager manages webhook subscriptions and event delivery.
type Manager struct {
	mu            sync.RWMutex
	subscriptions map[string]*Subscription
	client        *http.Client
	queue         chan *deliveryJob
	results       chan DeliveryResult
	retryPolicy   RetryPolicy
	workers       int
	ctx           context.Context
	cancel        context.CancelFunc
}

// RetryPolicy defines retry behavior for failed deliveries.
type RetryPolicy struct {
	MaxRetries    int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
}

// DefaultRetryPolicy returns the default retry policy.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:    3,
		InitialDelay:  time.Second,
		MaxDelay:      time.Minute,
		BackoffFactor: 2.0,
	}
}

type deliveryJob struct {
	subscription *Subscription
	event        *Event
	attempt      int
}

// NewManager creates a new webhook manager.
func NewManager(workers int, retryPolicy RetryPolicy) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		subscriptions: make(map[string]*Subscription),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		queue:       make(chan *deliveryJob, 1000),
		results:     make(chan DeliveryResult, 1000),
		retryPolicy: retryPolicy,
		workers:     workers,
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start workers
	for i := 0; i < workers; i++ {
		go m.worker()
	}

	return m
}

// Subscribe creates a new webhook subscription.
func (m *Manager) Subscribe(sub *Subscription) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sub.ID == "" {
		sub.ID = generateID()
	}
	sub.CreatedAt = time.Now()
	sub.Active = true

	m.subscriptions[sub.ID] = sub
	return nil
}

// Unsubscribe removes a webhook subscription.
func (m *Manager) Unsubscribe(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.subscriptions[id]; !exists {
		return fmt.Errorf("subscription not found: %s", id)
	}

	delete(m.subscriptions, id)
	return nil
}

// ListSubscriptions returns all subscriptions.
func (m *Manager) ListSubscriptions() []*Subscription {
	m.mu.RLock()
	defer m.mu.RUnlock()

	subs := make([]*Subscription, 0, len(m.subscriptions))
	for _, sub := range m.subscriptions {
		subs = append(subs, sub)
	}
	return subs
}

// GetSubscription returns a subscription by ID.
func (m *Manager) GetSubscription(id string) (*Subscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sub, exists := m.subscriptions[id]
	if !exists {
		return nil, fmt.Errorf("subscription not found: %s", id)
	}
	return sub, nil
}

// Emit sends an event to all matching subscriptions.
func (m *Manager) Emit(event *Event) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if event.ID == "" {
		event.ID = generateID()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	for _, sub := range m.subscriptions {
		if !sub.Active {
			continue
		}

		if !m.matchesSubscription(sub, event) {
			continue
		}

		// Queue for delivery
		select {
		case m.queue <- &deliveryJob{subscription: sub, event: event, attempt: 0}:
		default:
			// Queue full, log and continue
		}
	}
}

// EmitPointInsert emits a point insert event.
func (m *Manager) EmitPointInsert(collection, pointID string, payload map[string]interface{}) {
	m.Emit(&Event{
		Type: EventPointInsert,
		Data: map[string]interface{}{
			"collection": collection,
			"point_id":   pointID,
			"payload":    payload,
		},
	})
}

// EmitPointDelete emits a point delete event.
func (m *Manager) EmitPointDelete(collection, pointID string) {
	m.Emit(&Event{
		Type: EventPointDelete,
		Data: map[string]interface{}{
			"collection": collection,
			"point_id":   pointID,
		},
	})
}

// EmitCollectionCreate emits a collection create event.
func (m *Manager) EmitCollectionCreate(name string, dimension int) {
	m.Emit(&Event{
		Type: EventCollectionCreate,
		Data: map[string]interface{}{
			"name":      name,
			"dimension": dimension,
		},
	})
}

// EmitCollectionDelete emits a collection delete event.
func (m *Manager) EmitCollectionDelete(name string) {
	m.Emit(&Event{
		Type: EventCollectionDelete,
		Data: map[string]interface{}{
			"name": name,
		},
	})
}

func (m *Manager) matchesSubscription(sub *Subscription, event *Event) bool {
	// Check event type
	matched := false
	for _, et := range sub.Events {
		if et == event.Type {
			matched = true
			break
		}
	}
	if !matched {
		return false
	}

	// Check collection filter
	if sub.Collection != "" {
		if collection, ok := event.Data["collection"].(string); ok {
			if collection != sub.Collection {
				return false
			}
		}
	}

	return true
}

func (m *Manager) worker() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case job := <-m.queue:
			result := m.deliver(job)

			// Handle retry if failed
			if result.Error != nil && job.attempt < m.retryPolicy.MaxRetries {
				delay := m.calculateDelay(job.attempt)
				time.Sleep(delay)
				job.attempt++
				select {
				case m.queue <- job:
				default:
				}
			}

			// Send result
			select {
			case m.results <- result:
			default:
			}
		}
	}
}

func (m *Manager) deliver(job *deliveryJob) DeliveryResult {
	start := time.Now()

	payload, err := json.Marshal(job.event)
	if err != nil {
		return DeliveryResult{
			SubscriptionID: job.subscription.ID,
			EventID:        job.event.ID,
			Error:          err,
			Duration:       time.Since(start),
			Timestamp:      time.Now(),
		}
	}

	req, err := http.NewRequestWithContext(m.ctx, "POST", job.subscription.URL, bytes.NewReader(payload))
	if err != nil {
		return DeliveryResult{
			SubscriptionID: job.subscription.ID,
			EventID:        job.event.ID,
			Error:          err,
			Duration:       time.Since(start),
			Timestamp:      time.Now(),
		}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-LimyeDB-Event", string(job.event.Type))
	req.Header.Set("X-LimyeDB-Event-ID", job.event.ID)
	req.Header.Set("X-LimyeDB-Timestamp", job.event.Timestamp.Format(time.RFC3339))

	// Add signature if secret is configured
	if job.subscription.Secret != "" {
		signature := m.sign(payload, job.subscription.Secret)
		req.Header.Set("X-LimyeDB-Signature", signature)
	}

	// Add custom headers
	for k, v := range job.subscription.Headers {
		req.Header.Set(k, v)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return DeliveryResult{
			SubscriptionID: job.subscription.ID,
			EventID:        job.event.ID,
			Error:          err,
			Duration:       time.Since(start),
			Timestamp:      time.Now(),
		}
	}
	defer resp.Body.Close()

	result := DeliveryResult{
		SubscriptionID: job.subscription.ID,
		EventID:        job.event.ID,
		StatusCode:     resp.StatusCode,
		Duration:       time.Since(start),
		Timestamp:      time.Now(),
	}

	if resp.StatusCode >= 400 {
		result.Error = fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return result
}

func (m *Manager) sign(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

func (m *Manager) calculateDelay(attempt int) time.Duration {
	delay := m.retryPolicy.InitialDelay
	for i := 0; i < attempt; i++ {
		delay = time.Duration(float64(delay) * m.retryPolicy.BackoffFactor)
	}
	if delay > m.retryPolicy.MaxDelay {
		delay = m.retryPolicy.MaxDelay
	}
	return delay
}

// Results returns the channel for delivery results.
func (m *Manager) Results() <-chan DeliveryResult {
	return m.results
}

// Close shuts down the webhook manager.
func (m *Manager) Close() {
	m.cancel()
}

func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback: use time-based generation if crypto/rand fails
		for i := range b {
			b[i] = byte(time.Now().UnixNano() >> (i * 8)) //nolint:gosec // fallback only
		}
	}
	return hex.EncodeToString(b)
}
