package integrations

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	integrationv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/integration/v1"
)

type IntegrationSession struct {
	ID                string
	BotID             string
	ExternalSessionID string
	Metadata          map[string]string
	CreatedAt         time.Time
}

type Hub struct {
	mu          sync.Mutex
	connections map[string]*hubConnection
	events      []*integrationv1.IntegrationEvent
	acked       map[string]map[string]struct{}
	lastAcked   map[string]string
	sessions    map[string]IntegrationSession
}

type hubConnection struct {
	id            string
	tokenID       string
	send          func(*integrationv1.Envelope) error
	botIDs        map[string]struct{}
	botGroupIDs   map[string]struct{}
	eventTypes    map[string]struct{}
	connectedTime time.Time
}

func NewHub() *Hub {
	return &Hub{
		connections: make(map[string]*hubConnection),
		acked:       make(map[string]map[string]struct{}),
		lastAcked:   make(map[string]string),
		sessions:    make(map[string]IntegrationSession),
	}
}

func (h *Hub) Register(identity TokenIdentity, send func(*integrationv1.Envelope) error) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	id := uuid.NewString()
	h.connections[id] = &hubConnection{
		id:            id,
		tokenID:       identity.Token.ID,
		send:          send,
		botIDs:        make(map[string]struct{}),
		botGroupIDs:   make(map[string]struct{}),
		eventTypes:    make(map[string]struct{}),
		connectedTime: time.Now(),
	}
	return id
}

func (h *Hub) Unregister(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.connections, id)
}

func (h *Hub) Subscribe(connectionID string, botIDs, botGroupIDs, eventTypes []string) ([]*integrationv1.IntegrationEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	conn := h.connections[connectionID]
	if conn == nil {
		return nil, errors.New("integration connection not found")
	}
	conn.botIDs = stringSet(botIDs)
	conn.botGroupIDs = stringSet(botGroupIDs)
	conn.eventTypes = stringSet(eventTypes)
	replay := make([]*integrationv1.IntegrationEvent, 0)
	for _, event := range h.events {
		if h.shouldDeliverLocked(conn, event) {
			replay = append(replay, event)
		}
	}
	return replay, nil
}

func (h *Hub) Publish(event *integrationv1.IntegrationEvent) {
	if event == nil {
		return
	}
	if strings.TrimSpace(event.EventId) == "" {
		event.EventId = uuid.NewString()
	}
	if event.OccurredAt == nil {
		event.OccurredAt = timestamppb.Now()
	}
	h.mu.Lock()
	h.events = append(h.events, event)
	connections := make([]*hubConnection, 0, len(h.connections))
	for _, conn := range h.connections {
		if h.shouldDeliverLocked(conn, event) {
			connections = append(connections, conn)
		}
	}
	h.mu.Unlock()
	envelope := responseEnvelope("", &integrationv1.Envelope_Event{Event: event})
	for _, conn := range connections {
		_ = conn.send(envelope)
	}
}

func (h *Hub) Ack(tokenID, eventID string) bool {
	tokenID = strings.TrimSpace(tokenID)
	eventID = strings.TrimSpace(eventID)
	if tokenID == "" || eventID == "" {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.acked[tokenID] == nil {
		h.acked[tokenID] = make(map[string]struct{})
	}
	if _, ok := h.acked[tokenID][eventID]; ok {
		return false
	}
	h.acked[tokenID][eventID] = struct{}{}
	h.lastAcked[tokenID] = eventID
	return true
}

func (h *Hub) LastAckedEventID(tokenID string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastAcked[strings.TrimSpace(tokenID)]
}

func (h *Hub) CreateOrBindSession(tokenID, botID, externalSessionID string, metadata map[string]string) IntegrationSession {
	tokenID = strings.TrimSpace(tokenID)
	botID = strings.TrimSpace(botID)
	externalSessionID = strings.TrimSpace(externalSessionID)
	key := sessionKey(tokenID, botID, externalSessionID)
	h.mu.Lock()
	defer h.mu.Unlock()
	if externalSessionID != "" {
		if existing, ok := h.sessions[key]; ok {
			return existing
		}
	}
	session := IntegrationSession{
		ID:                uuid.NewString(),
		BotID:             botID,
		ExternalSessionID: externalSessionID,
		Metadata:          cloneStringMap(metadata),
		CreatedAt:         time.Now(),
	}
	h.sessions[key] = session
	h.sessions[session.ID] = session
	return session
}

func (h *Hub) Session(sessionID string) (IntegrationSession, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	session, ok := h.sessions[strings.TrimSpace(sessionID)]
	return session, ok
}

func (h *Hub) shouldDeliverLocked(conn *hubConnection, event *integrationv1.IntegrationEvent) bool {
	if conn == nil || event == nil {
		return false
	}
	if h.isAckedLocked(conn.tokenID, event.GetEventId()) {
		return false
	}
	if len(conn.eventTypes) > 0 {
		if _, ok := conn.eventTypes[event.GetEventType()]; !ok {
			return false
		}
	}
	if len(conn.botIDs) > 0 {
		if _, ok := conn.botIDs[event.GetBotId()]; ok {
			return true
		}
	}
	if len(conn.botGroupIDs) > 0 {
		if _, ok := conn.botGroupIDs[event.GetBotGroupId()]; ok {
			return true
		}
	}
	return len(conn.botIDs) == 0 && len(conn.botGroupIDs) == 0
}

func (h *Hub) isAckedLocked(tokenID, eventID string) bool {
	if h.acked[tokenID] == nil {
		return false
	}
	_, ok := h.acked[tokenID][eventID]
	return ok
}

func stringSet(items []string) map[string]struct{} {
	result := make(map[string]struct{}, len(items))
	for _, item := range items {
		if value := strings.TrimSpace(item); value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}

func sessionKey(tokenID, botID, externalSessionID string) string {
	if externalSessionID == "" {
		return uuid.NewString()
	}
	return tokenID + "\x00" + botID + "\x00" + externalSessionID
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
