package cluster

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// Transport handles node-to-node communication
type Transport interface {
	// Start starts the transport
	Start() error

	// Stop stops the transport
	Stop() error

	// Send sends a message to a node
	Send(ctx context.Context, nodeAddr string, msg *Message) (*Message, error)

	// Stream opens a bidirectional stream to a node
	Stream(ctx context.Context, nodeAddr string) (Stream, error)

	// OnMessage sets the message handler
	OnMessage(handler MessageHandler)
}

// Stream represents a bidirectional stream between nodes
type Stream interface {
	Send(msg *Message) error
	Recv() (*Message, error)
	Close() error
}

// MessageHandler handles incoming messages
type MessageHandler func(msg *Message) *Message

// MessageType represents the type of cluster message
type MessageType string

const (
	// Membership messages
	MsgTypeJoin      MessageType = "join"
	MsgTypeLeave     MessageType = "leave"
	MsgTypePing      MessageType = "ping"
	MsgTypePong      MessageType = "pong"
	MsgTypeGossip    MessageType = "gossip"

	// Raft messages
	MsgTypeRequestVote    MessageType = "request_vote"
	MsgTypeVoteResponse   MessageType = "vote_response"
	MsgTypeAppendEntries  MessageType = "append_entries"
	MsgTypeAppendResponse MessageType = "append_response"

	// Data messages
	MsgTypeForward      MessageType = "forward"
	MsgTypeReplicate    MessageType = "replicate"
	MsgTypeSnapshot     MessageType = "snapshot"
	MsgTypeStreamData   MessageType = "stream_data"
	MsgTypeRepairData   MessageType = "repair_data"

	// Search messages
	MsgTypeSearch       MessageType = "search"
	MsgTypeSearchResult MessageType = "search_result"
)

// Message represents a cluster message
type Message struct {
	Type      MessageType `json:"type"`
	From      string      `json:"from"`
	To        string      `json:"to"`
	Term      uint64      `json:"term,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   []byte      `json:"payload"`
}

// NewMessage creates a new message
func NewMessage(msgType MessageType, from, to string, payload interface{}) (*Message, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return &Message{
		Type:      msgType,
		From:      from,
		To:        to,
		Timestamp: time.Now(),
		Payload:   data,
	}, nil
}

// Decode decodes the message payload
func (m *Message) Decode(v interface{}) error {
	return json.Unmarshal(m.Payload, v)
}

// HTTPTransport implements Transport using HTTP
type HTTPTransport struct {
	addr     string
	server   *http.Server
	client   *http.Client
	handler  MessageHandler
	mu       sync.RWMutex
	listener net.Listener
}

// NewHTTPTransport creates a new HTTP transport
func NewHTTPTransport(addr string) *HTTPTransport {
	return &HTTPTransport{
		addr: addr,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (t *HTTPTransport) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/cluster/message", t.handleMessage)
	mux.HandleFunc("/cluster/stream", t.handleStream)

	t.server = &http.Server{
		Addr:         t.addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	listener, err := net.Listen("tcp", t.addr)
	if err != nil {
		return err
	}
	t.listener = listener

	go func() { _ = t.server.Serve(listener) }()
	return nil
}

func (t *HTTPTransport) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return t.server.Shutdown(ctx)
}

func (t *HTTPTransport) Send(ctx context.Context, nodeAddr string, msg *Message) (*Message, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("http://%s/cluster/message", nodeAddr)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response Message
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

func (t *HTTPTransport) Stream(ctx context.Context, nodeAddr string) (Stream, error) {
	// HTTP bidirectional streaming using Upgrade
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", nodeAddr)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "/cluster/stream", nil)
	if err != nil {
		_ = conn.Close() // Best effort close on error
		return nil, err
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "limyedb-stream")

	if err := req.Write(conn); err != nil {
		_ = conn.Close() // Best effort close on error
		return nil, err
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		_ = conn.Close() // Best effort close on error
		return nil, err
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		_ = conn.Close() // Best effort close on error
		return nil, fmt.Errorf("unexpected streaming status: %d", resp.StatusCode)
	}

	return &tcpStream{
		conn:    conn,
		encoder: json.NewEncoder(conn),
		decoder: json.NewDecoder(conn),
	}, nil
}

func (t *HTTPTransport) OnMessage(handler MessageHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.handler = handler
}

func (t *HTTPTransport) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var msg Message
	if err := json.Unmarshal(body, &msg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	t.mu.RLock()
	handler := t.handler
	t.mu.RUnlock()

	if handler == nil {
		http.Error(w, "no handler", http.StatusServiceUnavailable)
		return
	}

	response := handler(&msg)
	if response == nil {
		response = &Message{Type: "ack"}
	}

	responseData, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(responseData) // Error intentionally ignored for HTTP response writer
}

func (t *HTTPTransport) handleStream(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Upgrade") != "limyedb-stream" {
		http.Error(w, "Upgrade Required", http.StatusUpgradeRequired)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	conn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Write successful upgrade response manually since we hijacked the connection
	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Connection: Upgrade\r\n" +
		"Upgrade: limyedb-stream\r\n\r\n"
	
	if _, err := conn.Write([]byte(response)); err != nil {
		_ = conn.Close() // Best effort close on write error
		return
	}

	// We successfully grabbed the raw TCP conn! Treat it as a standard cluster stream.
	go func() {
		defer conn.Close()
		decoder := json.NewDecoder(conn)
		encoder := json.NewEncoder(conn)
		for {
			var msg Message
			if err := decoder.Decode(&msg); err != nil {
				return
			}
			t.mu.RLock()
			handler := t.handler
			t.mu.RUnlock()
			if handler != nil {
				if response := handler(&msg); response != nil {
					_ = encoder.Encode(response) // Best effort encode in streaming loop
				}
			}
		}
	}()
}

// TCPTransport implements Transport using TCP with message framing
type TCPTransport struct {
	addr     string
	listener net.Listener
	handler  MessageHandler
	conns    map[string]net.Conn
	mu       sync.RWMutex
	stopCh   chan struct{}
}

// NewTCPTransport creates a new TCP transport
func NewTCPTransport(addr string) *TCPTransport {
	return &TCPTransport{
		addr:   addr,
		conns:  make(map[string]net.Conn),
		stopCh: make(chan struct{}),
	}
}

func (t *TCPTransport) Start() error {
	listener, err := net.Listen("tcp", t.addr)
	if err != nil {
		return err
	}
	t.listener = listener

	go t.acceptLoop()
	return nil
}

func (t *TCPTransport) acceptLoop() {
	for {
		select {
		case <-t.stopCh:
			return
		default:
		}

		conn, err := t.listener.Accept()
		if err != nil {
			continue
		}

		go t.handleConn(conn)
	}
}

func (t *TCPTransport) handleConn(conn net.Conn) {
	defer conn.Close()

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	for {
		var msg Message
		if err := decoder.Decode(&msg); err != nil {
			return
		}

		t.mu.RLock()
		handler := t.handler
		t.mu.RUnlock()

		if handler != nil {
			response := handler(&msg)
			if response != nil {
				_ = encoder.Encode(response) // Best effort encode in streaming loop
			}
		}
	}
}

func (t *TCPTransport) Stop() error {
	close(t.stopCh)

	if t.listener != nil {
		_ = t.listener.Close() // Best effort close during shutdown
	}

	t.mu.Lock()
	for _, conn := range t.conns {
		_ = conn.Close() // Best effort close during shutdown
	}
	t.conns = make(map[string]net.Conn)
	t.mu.Unlock()

	return nil
}

func (t *TCPTransport) Send(ctx context.Context, nodeAddr string, msg *Message) (*Message, error) {
	conn, err := t.getOrCreateConn(nodeAddr)
	if err != nil {
		return nil, err
	}

	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	if err := encoder.Encode(msg); err != nil {
		t.removeConn(nodeAddr)
		return nil, err
	}

	var response Message
	if err := decoder.Decode(&response); err != nil {
		t.removeConn(nodeAddr)
		return nil, err
	}

	return &response, nil
}

func (t *TCPTransport) getOrCreateConn(addr string) (net.Conn, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if conn, ok := t.conns[addr]; ok {
		return conn, nil
	}

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}

	t.conns[addr] = conn
	return conn, nil
}

func (t *TCPTransport) removeConn(addr string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if conn, ok := t.conns[addr]; ok {
		_ = conn.Close()
		delete(t.conns, addr)
	}
}

func (t *TCPTransport) Stream(ctx context.Context, nodeAddr string) (Stream, error) {
	conn, err := net.DialTimeout("tcp", nodeAddr, 5*time.Second)
	if err != nil {
		return nil, err
	}

	return &tcpStream{
		conn:    conn,
		encoder: json.NewEncoder(conn),
		decoder: json.NewDecoder(conn),
	}, nil
}

func (t *TCPTransport) OnMessage(handler MessageHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.handler = handler
}

type tcpStream struct {
	conn    net.Conn
	encoder *json.Encoder
	decoder *json.Decoder
}

func (s *tcpStream) Send(msg *Message) error {
	return s.encoder.Encode(msg)
}

func (s *tcpStream) Recv() (*Message, error) {
	var msg Message
	if err := s.decoder.Decode(&msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (s *tcpStream) Close() error {
	return s.conn.Close()
}
