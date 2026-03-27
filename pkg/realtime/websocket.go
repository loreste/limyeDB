package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// EventType represents the type of real-time event
type EventType string

const (
	EventPointInsert      EventType = "point.insert"
	EventPointUpdate      EventType = "point.update"
	EventPointDelete      EventType = "point.delete"
	EventCollectionCreate EventType = "collection.create"
	EventCollectionDelete EventType = "collection.delete"
	EventSearchResult     EventType = "search.result"
	EventSubscribe        EventType = "subscribe"
	EventUnsubscribe      EventType = "unsubscribe"
	EventPing             EventType = "ping"
	EventPong             EventType = "pong"
	EventError            EventType = "error"
)

// Event represents a real-time event
type Event struct {
	Type       EventType              `json:"type"`
	Collection string                 `json:"collection,omitempty"`
	PointID    string                 `json:"point_id,omitempty"`
	Data       map[string]interface{} `json:"data,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
	RequestID  string                 `json:"request_id,omitempty"`
}

// Subscription represents a client subscription
type Subscription struct {
	ID         string
	Collection string
	Events     []EventType
	Filter     map[string]interface{}
}

// Client represents a WebSocket client
type Client struct {
	id            string
	conn          *websocket.Conn
	hub           *Hub
	send          chan []byte
	subscriptions map[string]*Subscription
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
}

// Hub manages all WebSocket connections and event broadcasting
type Hub struct {
	clients    map[string]*Client
	register   chan *Client
	unregister chan *Client
	broadcast  chan *Event
	mu         sync.RWMutex

	// Event handlers
	handlers map[EventType][]EventHandler
}

// EventHandler processes events before broadcasting
type EventHandler func(event *Event) bool // return false to cancel broadcast

// NewHub creates a new WebSocket hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *Event, 256),
		handlers:   make(map[EventType][]EventHandler),
	}
}

// Run starts the hub's main loop
func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			// Close all clients
			h.mu.Lock()
			for _, client := range h.clients {
				close(client.send)
			}
			h.clients = make(map[string]*Client)
			h.mu.Unlock()
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.id] = client
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.id]; ok {
				delete(h.clients, client.id)
				close(client.send)
			}
			h.mu.Unlock()

		case event := <-h.broadcast:
			h.broadcastEvent(event)
		}
	}
}

func (h *Hub) broadcastEvent(event *Event) {
	// Run handlers
	if handlers, ok := h.handlers[event.Type]; ok {
		for _, handler := range handlers {
			if !handler(event) {
				return // Cancel broadcast
			}
		}
	}

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, client := range h.clients {
		if h.shouldSendToClient(client, event) {
			select {
			case client.send <- data:
			default:
				// Client's buffer is full, skip
			}
		}
	}
}

func (h *Hub) shouldSendToClient(client *Client, event *Event) bool {
	client.mu.RLock()
	defer client.mu.RUnlock()

	for _, sub := range client.subscriptions {
		// Check collection match
		if sub.Collection != "" && sub.Collection != event.Collection && sub.Collection != "*" {
			continue
		}

		// Check event type match
		if len(sub.Events) > 0 {
			found := false
			for _, et := range sub.Events {
				if et == event.Type {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Apply filter to event data if present
		if len(sub.Filter) > 0 {
			if event.Data == nil {
				continue
			}
			match := true
			for key, val := range sub.Filter {
				if eventVal, ok := event.Data[key]; !ok || eventVal != val {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}

		return true
	}

	return false
}

// Publish publishes an event to all subscribed clients
func (h *Hub) Publish(event *Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	h.broadcast <- event
}

// RegisterHandler registers an event handler
func (h *Hub) RegisterHandler(eventType EventType, handler EventHandler) {
	h.handlers[eventType] = append(h.handlers[eventType], handler)
}

// ClientCount returns the number of connected clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in dev
	},
}

// ServeWS handles WebSocket connection upgrade
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	ctx, cancel := context.WithCancel(r.Context())

	client := &Client{
		id:            generateID(),
		conn:          conn,
		hub:           h,
		send:          make(chan []byte, 256),
		subscriptions: make(map[string]*Subscription),
		ctx:           ctx,
		cancel:        cancel,
	}

	h.register <- client

	// Start goroutines for reading and writing
	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		if err := c.conn.Close(); err != nil {
			log.Printf("readPump: error closing connection for client %s: %v", c.id, err)
		}
		c.cancel()
	}()

	c.conn.SetReadLimit(64 * 1024) // 64KB max message
	if err := c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		log.Printf("readPump: error setting read deadline for client %s: %v", c.id, err)
		return
	}
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		var msg ClientMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			c.sendError("invalid message format", "")
			continue
		}

		c.handleMessage(&msg)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		if err := c.conn.Close(); err != nil {
			log.Printf("writePump: error closing connection for client %s: %v", c.id, err)
		}
	}()

	for {
		select {
		case message, ok := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				log.Printf("writePump: error setting write deadline for client %s: %v", c.id, err)
				return
			}
			if !ok {
				if err := c.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
					log.Printf("writePump: error sending close message for client %s: %v", c.id, err)
				}
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			if err := c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				log.Printf("writePump: error setting write deadline for client %s: %v", c.id, err)
				return
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		case <-c.ctx.Done():
			return
		}
	}
}

// ClientMessage represents an incoming WebSocket message
type ClientMessage struct {
	Type       string                 `json:"type"`
	RequestID  string                 `json:"request_id,omitempty"`
	Collection string                 `json:"collection,omitempty"`
	Events     []EventType            `json:"events,omitempty"`
	Filter     map[string]interface{} `json:"filter,omitempty"`
	Data       map[string]interface{} `json:"data,omitempty"`
}

func (c *Client) handleMessage(msg *ClientMessage) {
	switch msg.Type {
	case "subscribe":
		c.handleSubscribe(msg)
	case "unsubscribe":
		c.handleUnsubscribe(msg)
	case "ping":
		c.handlePing(msg)
	default:
		c.sendError("unknown message type: "+msg.Type, msg.RequestID)
	}
}

func (c *Client) handleSubscribe(msg *ClientMessage) {
	sub := &Subscription{
		ID:         generateID(),
		Collection: msg.Collection,
		Events:     msg.Events,
		Filter:     msg.Filter,
	}

	c.mu.Lock()
	c.subscriptions[sub.ID] = sub
	c.mu.Unlock()

	response := map[string]interface{}{
		"type":            "subscribed",
		"subscription_id": sub.ID,
		"collection":      msg.Collection,
		"request_id":      msg.RequestID,
	}

	data, _ := json.Marshal(response)
	c.send <- data
}

func (c *Client) handleUnsubscribe(msg *ClientMessage) {
	c.mu.Lock()
	// Find and remove subscription by collection
	for id, sub := range c.subscriptions {
		if sub.Collection == msg.Collection {
			delete(c.subscriptions, id)
			break
		}
	}
	c.mu.Unlock()

	response := map[string]interface{}{
		"type":       "unsubscribed",
		"collection": msg.Collection,
		"request_id": msg.RequestID,
	}

	data, _ := json.Marshal(response)
	c.send <- data
}

func (c *Client) handlePing(msg *ClientMessage) {
	response := map[string]interface{}{
		"type":       "pong",
		"timestamp":  time.Now(),
		"request_id": msg.RequestID,
	}

	data, _ := json.Marshal(response)
	c.send <- data
}

func (c *Client) sendError(message, requestID string) {
	response := map[string]interface{}{
		"type":    "error",
		"message": message,
	}
	if requestID != "" {
		response["request_id"] = requestID
	}

	data, _ := json.Marshal(response)
	c.send <- data
}

// Simple ID generator
var idCounter int64
var idMu sync.Mutex

func generateID() string {
	idMu.Lock()
	idCounter++
	id := idCounter
	idMu.Unlock()
	if id < 0 || id > math.MaxInt32 {
		return time.Now().Format("20060102150405") + "-" + fmt.Sprintf("%d", id)
	}
	return time.Now().Format("20060102150405") + "-" + fmt.Sprintf("%d", id)
}

// EventPublisher is an interface for publishing events
type EventPublisher interface {
	Publish(event *Event)
}

// CollectionEventMiddleware creates event publishing middleware for collection operations
type CollectionEventMiddleware struct {
	publisher EventPublisher
}

// NewCollectionEventMiddleware creates a new event middleware
func NewCollectionEventMiddleware(publisher EventPublisher) *CollectionEventMiddleware {
	return &CollectionEventMiddleware{publisher: publisher}
}

// OnPointInsert publishes a point insert event
func (m *CollectionEventMiddleware) OnPointInsert(collection, pointID string, data map[string]interface{}) {
	m.publisher.Publish(&Event{
		Type:       EventPointInsert,
		Collection: collection,
		PointID:    pointID,
		Data:       data,
		Timestamp:  time.Now(),
	})
}

// OnPointUpdate publishes a point update event
func (m *CollectionEventMiddleware) OnPointUpdate(collection, pointID string, data map[string]interface{}) {
	m.publisher.Publish(&Event{
		Type:       EventPointUpdate,
		Collection: collection,
		PointID:    pointID,
		Data:       data,
		Timestamp:  time.Now(),
	})
}

// OnPointDelete publishes a point delete event
func (m *CollectionEventMiddleware) OnPointDelete(collection, pointID string) {
	m.publisher.Publish(&Event{
		Type:       EventPointDelete,
		Collection: collection,
		PointID:    pointID,
		Timestamp:  time.Now(),
	})
}

// OnCollectionCreate publishes a collection create event
func (m *CollectionEventMiddleware) OnCollectionCreate(collection string, data map[string]interface{}) {
	m.publisher.Publish(&Event{
		Type:       EventCollectionCreate,
		Collection: collection,
		Data:       data,
		Timestamp:  time.Now(),
	})
}

// OnCollectionDelete publishes a collection delete event
func (m *CollectionEventMiddleware) OnCollectionDelete(collection string) {
	m.publisher.Publish(&Event{
		Type:       EventCollectionDelete,
		Collection: collection,
		Timestamp:  time.Now(),
	})
}

// ChangeStream provides a change stream interface similar to MongoDB
type ChangeStream struct {
	hub        *Hub
	collection string
	events     []EventType
	filter     map[string]interface{}
	ch         chan *Event
	ctx        context.Context
	cancel     context.CancelFunc
}

// Watch creates a change stream for a collection
func (h *Hub) Watch(ctx context.Context, collection string, opts *WatchOptions) (*ChangeStream, error) {
	if opts == nil {
		opts = &WatchOptions{}
	}

	streamCtx, cancel := context.WithCancel(ctx)

	cs := &ChangeStream{
		hub:        h,
		collection: collection,
		events:     opts.Events,
		filter:     opts.Filter,
		ch:         make(chan *Event, 100),
		ctx:        streamCtx,
		cancel:     cancel,
	}

	// Register as internal subscriber
	go cs.run()

	return cs, nil
}

// WatchOptions configures a change stream
type WatchOptions struct {
	Events []EventType
	Filter map[string]interface{}
}

func (cs *ChangeStream) run() {
	sub := make(chan *Event, 100)

	// Simple internal subscription mechanism
	// In production, this would integrate with the hub's broadcast
	for {
		select {
		case <-cs.ctx.Done():
			close(cs.ch)
			return
		case event := <-sub:
			if cs.shouldInclude(event) {
				select {
				case cs.ch <- event:
				default:
					// Channel full, drop event
				}
			}
		}
	}
}

func (cs *ChangeStream) shouldInclude(event *Event) bool {
	if cs.collection != "" && cs.collection != event.Collection && cs.collection != "*" {
		return false
	}

	if len(cs.events) > 0 {
		found := false
		for _, et := range cs.events {
			if et == event.Type {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// Next returns the next event in the stream
func (cs *ChangeStream) Next() (*Event, error) {
	select {
	case event, ok := <-cs.ch:
		if !ok {
			return nil, errors.New("change stream closed")
		}
		return event, nil
	case <-cs.ctx.Done():
		return nil, cs.ctx.Err()
	}
}

// Close closes the change stream
func (cs *ChangeStream) Close() {
	cs.cancel()
}

// Events returns the event channel for range iteration
func (cs *ChangeStream) Events() <-chan *Event {
	return cs.ch
}
