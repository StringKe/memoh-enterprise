package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/encoding/protojson"

	integrationv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/integration/v1"
)

const defaultProtocolVersion = "2026-05-05"

type Options struct {
	URL                  string
	Token                string
	ClientID             string
	ClientCredential     string
	ProtocolVersion      string
	RequestTimeout       time.Duration
	HeartbeatInterval    time.Duration
	ReconnectBackoff     time.Duration
	MaxReconnectAttempts int
	Dialer               *websocket.Dialer
	Header               http.Header
	IDFactory            func() string
}

type Client struct {
	options Options
	conn    *websocket.Conn
	events  chan *integrationv1.Envelope
	done    chan struct{}
	pending map[string]chan response
	seen    map[string]struct{}
	mu      sync.Mutex
	writeMu sync.Mutex
}

type ConnectionInfo struct {
	IntegrationID   string
	ScopeType       string
	ScopeBotID      string
	ScopeBotGroupID string
}

type SubscribeOptions struct {
	EventTypes  []string
	BotIDs      []string
	BotGroupIDs []string
}

type SendBotMessageOptions struct {
	BotID     string
	SessionID string
	Text      string
	Metadata  map[string]string
}

type CreateSessionOptions struct {
	BotID             string
	ExternalSessionID string
	Metadata          map[string]string
}

type RequestActionOptions struct {
	BotID       string
	ActionType  string
	PayloadJSON string
	Metadata    map[string]string
}

type ProtocolError struct {
	Code    string
	Message string
}

type response struct {
	envelope *integrationv1.Envelope
	err      error
}

func (e *ProtocolError) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

func New(options Options) *Client {
	if options.ProtocolVersion == "" {
		options.ProtocolVersion = defaultProtocolVersion
	}
	if options.RequestTimeout == 0 {
		options.RequestTimeout = 10 * time.Second
	}
	if options.ReconnectBackoff == 0 {
		options.ReconnectBackoff = 500 * time.Millisecond
	}
	if options.Dialer == nil {
		options.Dialer = websocket.DefaultDialer
	}
	if options.IDFactory == nil {
		options.IDFactory = newID
	}
	return &Client{
		options: options,
		events:  make(chan *integrationv1.Envelope, 64),
		done:    make(chan struct{}),
		pending: make(map[string]chan response),
		seen:    make(map[string]struct{}),
	}
}

func (c *Client) Connect(ctx context.Context) (ConnectionInfo, error) {
	conn, resp, err := c.options.Dialer.DialContext(ctx, c.options.URL, c.dialHeader())
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return ConnectionInfo{}, err
	}
	authenticated := false
	defer func() {
		if authenticated {
			return
		}
		c.mu.Lock()
		if c.conn == conn {
			c.conn = nil
		}
		c.mu.Unlock()
		_ = conn.Close()
	}()
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	if err := c.writeEnvelope(&integrationv1.Envelope{
		Version:   c.options.ProtocolVersion,
		MessageId: c.options.IDFactory(),
		Payload: &integrationv1.Envelope_AuthRequest{
			AuthRequest: &integrationv1.AuthRequest{Token: c.options.Token},
		},
	}); err != nil {
		return ConnectionInfo{}, err
	}
	envelope, err := readEnvelope(conn)
	if err != nil {
		return ConnectionInfo{}, err
	}
	if protocolErr := envelope.GetError(); protocolErr != nil {
		return ConnectionInfo{}, &ProtocolError{Code: protocolErr.GetCode(), Message: protocolErr.GetMessage()}
	}
	auth := envelope.GetAuthResponse()
	if auth == nil {
		return ConnectionInfo{}, errors.New("integration auth response missing")
	}
	c.resetRuntime()
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	authenticated = true
	go c.readLoop(conn)
	if c.options.HeartbeatInterval > 0 {
		go c.heartbeatLoop(context.WithoutCancel(ctx), c.done)
	}
	return ConnectionInfo{
		IntegrationID:   auth.GetIntegrationId(),
		ScopeType:       auth.GetScopeType(),
		ScopeBotID:      auth.GetScopeBotId(),
		ScopeBotGroupID: auth.GetScopeBotGroupId(),
	}, nil
}

func (c *Client) Reconnect(ctx context.Context) (ConnectionInfo, error) {
	var lastErr error
	for attempt := 0; attempt <= c.options.MaxReconnectAttempts; attempt++ {
		info, err := c.Connect(ctx)
		if err == nil {
			return info, nil
		}
		lastErr = err
		if attempt == c.options.MaxReconnectAttempts {
			break
		}
		timer := time.NewTimer(c.options.ReconnectBackoff * time.Duration(attempt+1))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ConnectionInfo{}, ctx.Err()
		case <-timer.C:
		}
	}
	return ConnectionInfo{}, lastErr
}

func (c *Client) Events() <-chan *integrationv1.Envelope {
	return c.events
}

func (c *Client) Subscribe(ctx context.Context, options SubscribeOptions) (*integrationv1.SubscribeResponse, error) {
	envelope, err := c.request(ctx, &integrationv1.Envelope_SubscribeRequest{
		SubscribeRequest: &integrationv1.SubscribeRequest{
			EventTypes:  append([]string(nil), options.EventTypes...),
			BotIds:      append([]string(nil), options.BotIDs...),
			BotGroupIds: append([]string(nil), options.BotGroupIDs...),
		},
	})
	if err != nil {
		return nil, err
	}
	res := envelope.GetSubscribeResponse()
	if res == nil {
		return nil, errors.New("integration subscribe response missing")
	}
	return res, nil
}

func (c *Client) AckEvent(ctx context.Context, eventID string) error {
	envelope, err := c.request(ctx, &integrationv1.Envelope_AckRequest{
		AckRequest: &integrationv1.AckRequest{EventId: eventID},
	})
	if err != nil {
		return err
	}
	res := envelope.GetAckResponse()
	if res == nil {
		return errors.New("integration ack response missing")
	}
	if res.GetEventId() != eventID {
		return fmt.Errorf("integration ack event mismatch: %s", res.GetEventId())
	}
	return nil
}

func (c *Client) Ping(ctx context.Context) error {
	envelope, err := c.request(ctx, &integrationv1.Envelope_Ping{Ping: &integrationv1.Ping{}})
	if err != nil {
		return err
	}
	if envelope.GetPong() == nil {
		return errors.New("integration pong response missing")
	}
	return nil
}

func (c *Client) SendBotMessage(ctx context.Context, options SendBotMessageOptions) (*integrationv1.SendBotMessageResponse, error) {
	envelope, err := c.request(ctx, &integrationv1.Envelope_SendBotMessageRequest{
		SendBotMessageRequest: &integrationv1.SendBotMessageRequest{
			BotId:     options.BotID,
			SessionId: options.SessionID,
			Text:      options.Text,
			Metadata:  cloneStringMap(options.Metadata),
		},
	})
	if err != nil {
		return nil, err
	}
	res := envelope.GetSendBotMessageResponse()
	if res == nil {
		return nil, errors.New("integration send bot message response missing")
	}
	return res, nil
}

func (c *Client) CreateSession(ctx context.Context, options CreateSessionOptions) (*integrationv1.CreateSessionResponse, error) {
	envelope, err := c.request(ctx, &integrationv1.Envelope_CreateSessionRequest{
		CreateSessionRequest: &integrationv1.CreateSessionRequest{
			BotId:             options.BotID,
			ExternalSessionId: options.ExternalSessionID,
			Metadata:          cloneStringMap(options.Metadata),
		},
	})
	if err != nil {
		return nil, err
	}
	res := envelope.GetCreateSessionResponse()
	if res == nil {
		return nil, errors.New("integration create session response missing")
	}
	return res, nil
}

func (c *Client) GetSessionStatus(ctx context.Context, sessionID string) (*integrationv1.GetSessionStatusResponse, error) {
	envelope, err := c.request(ctx, &integrationv1.Envelope_GetSessionStatusRequest{
		GetSessionStatusRequest: &integrationv1.GetSessionStatusRequest{SessionId: sessionID},
	})
	if err != nil {
		return nil, err
	}
	res := envelope.GetGetSessionStatusResponse()
	if res == nil {
		return nil, errors.New("integration get session status response missing")
	}
	return res, nil
}

func (c *Client) GetBotStatus(ctx context.Context, botID string) (*integrationv1.GetBotStatusResponse, error) {
	envelope, err := c.request(ctx, &integrationv1.Envelope_GetBotStatusRequest{
		GetBotStatusRequest: &integrationv1.GetBotStatusRequest{BotId: botID},
	})
	if err != nil {
		return nil, err
	}
	res := envelope.GetGetBotStatusResponse()
	if res == nil {
		return nil, errors.New("integration get bot status response missing")
	}
	return res, nil
}

func (c *Client) RequestAction(ctx context.Context, options RequestActionOptions) (*integrationv1.RequestActionResponse, error) {
	envelope, err := c.request(ctx, &integrationv1.Envelope_RequestActionRequest{
		RequestActionRequest: &integrationv1.RequestActionRequest{
			BotId:       options.BotID,
			ActionType:  options.ActionType,
			PayloadJson: options.PayloadJSON,
			Metadata:    cloneStringMap(options.Metadata),
		},
	})
	if err != nil {
		return nil, err
	}
	res := envelope.GetRequestActionResponse()
	if res == nil {
		return nil, errors.New("integration request action response missing")
	}
	return res, nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.rejectPendingLocked(errors.New("integration websocket closed"))
	c.closeDoneLocked()
	c.mu.Unlock()
	if conn == nil {
		return nil
	}
	return conn.Close()
}

func (c *Client) request(ctx context.Context, payload any) (*integrationv1.Envelope, error) {
	id := c.options.IDFactory()
	ch := make(chan response, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()
	envelope := &integrationv1.Envelope{
		Version:       c.options.ProtocolVersion,
		MessageId:     id,
		CorrelationId: id,
	}
	switch value := payload.(type) {
	case *integrationv1.Envelope_SubscribeRequest:
		envelope.Payload = value
	case *integrationv1.Envelope_AckRequest:
		envelope.Payload = value
	case *integrationv1.Envelope_Ping:
		envelope.Payload = value
	case *integrationv1.Envelope_SendBotMessageRequest:
		envelope.Payload = value
	case *integrationv1.Envelope_CreateSessionRequest:
		envelope.Payload = value
	case *integrationv1.Envelope_GetSessionStatusRequest:
		envelope.Payload = value
	case *integrationv1.Envelope_GetBotStatusRequest:
		envelope.Payload = value
	case *integrationv1.Envelope_RequestActionRequest:
		envelope.Payload = value
	default:
		return nil, fmt.Errorf("unsupported integration request payload %T", payload)
	}
	if err := c.writeEnvelope(envelope); err != nil {
		return nil, err
	}
	timer := time.NewTimer(c.options.RequestTimeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, errors.New("integration request timed out")
	case res := <-ch:
		return res.envelope, res.err
	}
}

func (c *Client) readLoop(conn *websocket.Conn) {
	for {
		envelope, err := readEnvelope(conn)
		if err != nil {
			c.mu.Lock()
			if c.conn == conn {
				c.conn = nil
				c.rejectPendingLocked(err)
				c.closeDoneLocked()
			}
			c.mu.Unlock()
			return
		}
		if c.dispatchPending(envelope) {
			continue
		}
		if c.seenEvent(envelope.GetMessageId()) {
			continue
		}
		select {
		case c.events <- envelope:
		default:
		}
	}
}

func (c *Client) dispatchPending(envelope *integrationv1.Envelope) bool {
	correlationID := envelope.GetCorrelationId()
	if correlationID == "" {
		return false
	}
	c.mu.Lock()
	ch := c.pending[correlationID]
	if ch != nil {
		delete(c.pending, correlationID)
	}
	c.mu.Unlock()
	if ch == nil {
		return false
	}
	if protocolErr := envelope.GetError(); protocolErr != nil {
		ch <- response{err: &ProtocolError{Code: protocolErr.GetCode(), Message: protocolErr.GetMessage()}}
		return true
	}
	ch <- response{envelope: envelope}
	return true
}

func (c *Client) resetRuntime() {
	c.mu.Lock()
	c.pending = make(map[string]chan response)
	c.seen = make(map[string]struct{})
	c.events = make(chan *integrationv1.Envelope, 64)
	c.done = make(chan struct{})
	c.mu.Unlock()
}

func (c *Client) heartbeatLoop(ctx context.Context, done <-chan struct{}) {
	ticker := time.NewTicker(c.options.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, c.options.RequestTimeout)
			err := c.Ping(pingCtx)
			cancel()
			if err != nil {
				_ = c.Close()
				return
			}
		}
	}
}

func (c *Client) seenEvent(messageID string) bool {
	if messageID == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.seen[messageID]; ok {
		return true
	}
	c.seen[messageID] = struct{}{}
	return false
}

func (c *Client) rejectPendingLocked(err error) {
	for id, ch := range c.pending {
		ch <- response{err: err}
		delete(c.pending, id)
	}
}

func (c *Client) closeDoneLocked() {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
}

func (c *Client) dialHeader() http.Header {
	header := cloneHeader(c.options.Header)
	if c.options.Token != "" && !hasHeader(header, "Authorization") {
		header.Set("Authorization", "Bearer "+c.options.Token)
	}
	if c.options.ClientID != "" {
		header.Set("X-Memoh-Client-ID", c.options.ClientID)
	}
	if c.options.ClientCredential != "" {
		header.Set("X-Memoh-Client-Secret", c.options.ClientCredential)
	}
	return header
}

func (c *Client) writeEnvelope(envelope *integrationv1.Envelope) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return errors.New("integration websocket is not connected")
	}
	payload, err := protojson.MarshalOptions{EmitUnpopulated: false}.Marshal(envelope)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, payload)
}

func readEnvelope(conn *websocket.Conn) (*integrationv1.Envelope, error) {
	messageType, payload, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
		return nil, errors.New("unsupported integration websocket message type")
	}
	var envelope integrationv1.Envelope
	if err := protojson.Unmarshal(payload, &envelope); err != nil {
		return nil, err
	}
	return &envelope, nil
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(b[:])
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneHeader(header http.Header) http.Header {
	if len(header) == 0 {
		return make(http.Header)
	}
	cloned := make(http.Header, len(header))
	for key, values := range header {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}

func hasHeader(header http.Header, name string) bool {
	if len(header) == 0 {
		return false
	}
	_, ok := header[http.CanonicalHeaderKey(name)]
	if ok {
		return true
	}
	for key := range header {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}
