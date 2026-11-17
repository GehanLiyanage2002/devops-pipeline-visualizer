package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pipescope/backend/internal/graph"
)

// TelemetryEvent is broadcast over /ws/telemetry whenever node statuses, edge animations,
// or pipeline graph updates occur during a simulation or webhook event.
type TelemetryEvent struct {
	Type          string                      `json:"type"`
	Timestamp     string                      `json:"timestamp"`
	Stage         string                      `json:"stage,omitempty"`
	Message       string                      `json:"message,omitempty"`
	NodeStatuses  map[string]graph.NodeStatus `json:"node_statuses"`
	AnimatedEdges []string                    `json:"animated_edges"`
	Graph         *graph.PipelineGraph        `json:"graph,omitempty"`
}

// TelemetryHub manages active WebSocket clients and broadcasts real-time pipeline telemetry.
type TelemetryHub struct {
	mu       sync.Mutex
	clients  map[*websocket.Conn]struct{}
	upgrader websocket.Upgrader
}

// NewTelemetryHub creates and initializes a new TelemetryHub.
func NewTelemetryHub() *TelemetryHub {
	return &TelemetryHub{
		clients: make(map[*websocket.Conn]struct{}),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

// HandleWebSocket upgrades an HTTP GET request on /ws/telemetry to a WebSocket connection.
func (h *TelemetryHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	h.mu.Lock()
	h.clients[conn] = struct{}{}
	h.mu.Unlock()

	// Send initial connection ack
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_ = conn.WriteJSON(TelemetryEvent{
		Type:          "connected",
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Message:       "Connected to PipeScope telemetry stream",
		NodeStatuses:  map[string]graph.NodeStatus{},
		AnimatedEdges: []string{},
	})

	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.clients, conn)
			h.mu.Unlock()
			_ = conn.Close()
		}()

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}()
}

// Broadcast sends a TelemetryEvent to all connected WebSocket clients.
func (h *TelemetryHub) Broadcast(event TelemetryEvent) {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for conn := range h.clients {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			_ = conn.Close()
			delete(h.clients, conn)
		}
	}
}
