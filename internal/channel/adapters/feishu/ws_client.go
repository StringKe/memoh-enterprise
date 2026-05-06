package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

const (
	feishuWSDefaultReconnectInterval = 3 * time.Second
	feishuWSDefaultPingInterval      = 2 * time.Minute
	feishuWSFragmentTTL              = 5 * time.Second
)

type feishuWSClient struct {
	appID        string
	appSecret    string
	baseURL      string
	dispatcher   *dispatcher.EventDispatcher
	httpClient   *http.Client
	dialer       *websocket.Dialer
	logger       *slog.Logger
	fragments    map[string]feishuWSFragment
	reconnectCnt int

	mu                sync.Mutex
	conn              *websocket.Conn
	serviceID         string
	reconnectInterval time.Duration
	reconnectNonce    time.Duration
	pingInterval      time.Duration
}

type feishuWSFragment struct {
	parts     [][]byte
	expiresAt time.Time
}

type feishuWSOption func(*feishuWSClient)

func withFeishuWSBaseURL(baseURL string) feishuWSOption {
	return func(c *feishuWSClient) {
		c.baseURL = baseURL
	}
}

func newFeishuWSClient(cfg Config, eventDispatcher *dispatcher.EventDispatcher, logger *slog.Logger, opts ...feishuWSOption) *feishuWSClient {
	client := &feishuWSClient{
		appID:             cfg.AppID,
		appSecret:         cfg.AppSecret,
		baseURL:           cfg.openBaseURL(),
		dispatcher:        eventDispatcher,
		httpClient:        http.DefaultClient,
		dialer:            websocket.DefaultDialer,
		logger:            logger,
		fragments:         map[string]feishuWSFragment{},
		reconnectCnt:      -1,
		reconnectInterval: feishuWSDefaultReconnectInterval,
		pingInterval:      feishuWSDefaultPingInterval,
	}
	for _, opt := range opts {
		opt(client)
	}
	return client
}

func (c *feishuWSClient) Run(ctx context.Context) error {
	attempt := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		err := c.connect(ctx)
		if err == nil {
			attempt = 0
			err = c.runConnected(ctx)
		}
		c.closeConn()
		if ctx.Err() != nil {
			return nil
		}
		if err != nil && c.logger != nil {
			c.logger.Warn("feishu websocket disconnected", slog.Any("error", err))
		}
		if !c.shouldRetry(attempt) {
			if err != nil {
				return err
			}
			return errors.New("feishu websocket stopped before reconnect")
		}
		delay := c.nextReconnectDelay(attempt)
		if !sleepContext(ctx, delay) {
			return nil
		}
		attempt++
	}
}

func (c *feishuWSClient) Close() error {
	c.closeConn()
	return nil
}

func (c *feishuWSClient) connect(ctx context.Context) error {
	connURL, err := c.getConnURL(ctx)
	if err != nil {
		return err
	}
	u, err := url.Parse(connURL)
	if err != nil {
		return err
	}
	conn, resp, err := c.dialer.DialContext(ctx, connURL, nil)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
			return parseFeishuWSHandshakeError(resp)
		}
		return err
	}
	if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
		_ = conn.Close()
		return parseFeishuWSHandshakeError(resp)
	}

	c.mu.Lock()
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn = conn
	c.serviceID = u.Query().Get(larkws.ServiceID)
	c.mu.Unlock()
	if c.logger != nil {
		c.logger.Info("feishu websocket connected", slog.String("url", u.Redacted()))
	}
	return nil
}

func (c *feishuWSClient) getConnURL(ctx context.Context) (string, error) {
	body := map[string]string{
		"AppID":     c.appID,
		"AppSecret": c.appSecret,
	}
	bs, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+larkws.GenEndpointUri, bytes.NewBuffer(bs))
	if err != nil {
		return "", err
	}
	req.Header.Add("locale", "zh")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", larkws.NewServerError(resp.StatusCode, "system busy")
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	endpointResp := &larkws.EndpointResp{}
	if err := json.Unmarshal(respBody, endpointResp); err != nil {
		return "", err
	}
	switch endpointResp.Code {
	case larkws.OK:
	case larkws.SystemBusy:
		return "", larkws.NewServerError(endpointResp.Code, "system busy")
	case larkws.InternalError:
		return "", larkws.NewServerError(endpointResp.Code, endpointResp.Msg)
	default:
		return "", larkws.NewClientError(endpointResp.Code, endpointResp.Msg)
	}
	if endpointResp.Data == nil || endpointResp.Data.Url == "" {
		return "", larkws.NewServerError(http.StatusInternalServerError, "endpoint is null")
	}
	if endpointResp.Data.ClientConfig != nil {
		c.configure(endpointResp.Data.ClientConfig)
	}
	return endpointResp.Data.Url, nil
}

func (c *feishuWSClient) runConnected(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	pingDone := make(chan struct{})
	go func() {
		defer close(pingDone)
		c.pingLoop(runCtx)
	}()
	err := c.receiveMessageLoop(runCtx)
	cancel()
	c.closeConn()
	<-pingDone
	return err
}

func (c *feishuWSClient) pingLoop(ctx context.Context) {
	for {
		if err := c.writePing(); err != nil && c.logger != nil && ctx.Err() == nil {
			c.logger.Warn("feishu websocket ping failed", slog.Any("error", err))
		}
		interval := c.currentPingInterval()
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (c *feishuWSClient) writePing() error {
	serviceID := c.currentServiceID()
	frame := larkws.NewPingFrame(serviceID)
	return c.writeFrame(websocket.BinaryMessage, frame)
}

func (c *feishuWSClient) receiveMessageLoop(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		conn := c.currentConn()
		if conn == nil {
			return errors.New("feishu websocket connection is closed")
		}
		messageType, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		if messageType != websocket.BinaryMessage {
			if c.logger != nil {
				c.logger.Warn("feishu websocket ignored non-binary message", slog.Int("message_type", messageType))
			}
			continue
		}
		go c.handleMessage(ctx, msg)
	}
}

func (c *feishuWSClient) handleMessage(ctx context.Context, msg []byte) {
	var frame larkws.Frame
	if err := frame.Unmarshal(msg); err != nil {
		if c.logger != nil {
			c.logger.Error("feishu websocket unmarshal frame failed", slog.Any("error", err))
		}
		return
	}
	switch larkws.FrameType(frame.Method) {
	case larkws.FrameTypeControl:
		c.handleControlFrame(&frame)
	case larkws.FrameTypeData:
		c.handleDataFrame(ctx, &frame)
	}
}

func (c *feishuWSClient) handleControlFrame(frame *larkws.Frame) {
	hs := larkws.Headers(frame.Headers)
	if larkws.MessageType(hs.GetString(larkws.HeaderType)) != larkws.MessageTypePong {
		return
	}
	if len(frame.Payload) == 0 {
		return
	}
	cfg := &larkws.ClientConfig{}
	if err := json.Unmarshal(frame.Payload, cfg); err != nil {
		if c.logger != nil {
			c.logger.Warn("feishu websocket unmarshal pong config failed", slog.Any("error", err))
		}
		return
	}
	c.configure(cfg)
}

func (c *feishuWSClient) handleDataFrame(ctx context.Context, frame *larkws.Frame) {
	hs := larkws.Headers(frame.Headers)
	sum := hs.GetInt(larkws.HeaderSum)
	seq := hs.GetInt(larkws.HeaderSeq)
	msgID := hs.GetString(larkws.HeaderMessageID)
	msgType := larkws.MessageType(hs.GetString(larkws.HeaderType))
	payload := frame.Payload
	if sum > 1 {
		payload = c.combine(msgID, sum, seq, payload)
		if payload == nil {
			return
		}
	}

	var respPayload []byte
	resp := larkws.NewResponseByCode(http.StatusOK)
	started := time.Now()
	switch msgType {
	case larkws.MessageTypeEvent:
		var rsp any
		var err error
		if c.dispatcher == nil {
			err = errors.New("feishu websocket event dispatcher is not configured")
		} else {
			rsp, err = c.dispatcher.Do(ctx, payload)
		}
		if err != nil {
			if c.logger != nil {
				c.logger.Error("feishu websocket handle event failed", slog.Any("error", err))
			}
			resp = larkws.NewResponseByCode(http.StatusInternalServerError)
		} else if rsp != nil {
			var err error
			resp.Data, err = json.Marshal(rsp)
			if err != nil {
				if c.logger != nil {
					c.logger.Error("feishu websocket marshal response failed", slog.Any("error", err))
				}
				resp = larkws.NewResponseByCode(http.StatusInternalServerError)
			}
		}
	case larkws.MessageTypeCard:
		return
	default:
		return
	}
	hs.Add(larkws.HeaderBizRt, strconv.FormatInt(time.Since(started).Milliseconds(), 10))
	var err error
	respPayload, err = json.Marshal(resp)
	if err != nil {
		if c.logger != nil {
			c.logger.Error("feishu websocket marshal ack failed", slog.Any("error", err))
		}
		return
	}
	frame.Payload = respPayload
	frame.Headers = hs
	if err := c.writeFrame(websocket.BinaryMessage, frame); err != nil && c.logger != nil {
		c.logger.Error("feishu websocket write ack failed", slog.Any("error", err))
	}
}

func (c *feishuWSClient) combine(msgID string, sum int, seq int, bs []byte) []byte {
	if msgID == "" || sum <= 1 || seq < 0 || seq >= sum {
		return bs
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, fragment := range c.fragments {
		if now.After(fragment.expiresAt) {
			delete(c.fragments, id)
		}
	}
	fragment, ok := c.fragments[msgID]
	if !ok || len(fragment.parts) != sum {
		fragment = feishuWSFragment{
			parts:     make([][]byte, sum),
			expiresAt: now.Add(feishuWSFragmentTTL),
		}
	}
	fragment.parts[seq] = bs
	capacity := 0
	for _, part := range fragment.parts {
		if len(part) == 0 {
			fragment.expiresAt = now.Add(feishuWSFragmentTTL)
			c.fragments[msgID] = fragment
			return nil
		}
		capacity += len(part)
	}
	delete(c.fragments, msgID)
	merged := make([]byte, 0, capacity)
	for _, part := range fragment.parts {
		merged = append(merged, part...)
	}
	return merged
}

func (c *feishuWSClient) writeFrame(messageType int, frame *larkws.Frame) error {
	bs, err := frame.Marshal()
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return errors.New("feishu websocket connection is closed")
	}
	return c.conn.WriteMessage(messageType, bs)
}

func (c *feishuWSClient) closeConn() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return
	}
	_ = c.conn.Close()
	c.conn = nil
	c.serviceID = ""
}

func (c *feishuWSClient) currentConn() *websocket.Conn {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn
}

func (c *feishuWSClient) currentServiceID() int32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	serviceID, _ := strconv.ParseInt(c.serviceID, 10, 32)
	return int32(serviceID)
}

func (c *feishuWSClient) currentPingInterval() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pingInterval
}

func (c *feishuWSClient) shouldRetry(attempt int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reconnectCnt < 0 || attempt < c.reconnectCnt
}

func (c *feishuWSClient) nextReconnectDelay(attempt int) time.Duration {
	c.mu.Lock()
	interval := c.reconnectInterval
	nonce := c.reconnectNonce
	c.mu.Unlock()
	if attempt == 0 && nonce > 0 {
		return nonce
	}
	return interval
}

func (c *feishuWSClient) configure(cfg *larkws.ClientConfig) {
	if cfg == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reconnectCnt = cfg.ReconnectCount
	if cfg.ReconnectInterval > 0 {
		c.reconnectInterval = time.Duration(cfg.ReconnectInterval) * time.Second
	}
	if cfg.ReconnectNonce > 0 {
		c.reconnectNonce = time.Duration(cfg.ReconnectNonce) * time.Second
	}
	if cfg.PingInterval > 0 {
		c.pingInterval = time.Duration(cfg.PingInterval) * time.Second
	}
}

func sleepContext(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func parseFeishuWSHandshakeError(resp *http.Response) error {
	code, _ := strconv.Atoi(resp.Header.Get(larkws.HeaderHandshakeStatus))
	msg := resp.Header.Get(larkws.HeaderHandshakeMsg)
	switch code {
	case larkws.AuthFailed:
		authCode, _ := strconv.Atoi(resp.Header.Get(larkws.HeaderHandshakeAuthErrCode))
		if authCode == larkws.ExceedConnLimit {
			return larkws.NewClientError(code, msg)
		}
		return larkws.NewServerError(code, msg)
	case larkws.Forbidden:
		return larkws.NewClientError(code, msg)
	default:
		return larkws.NewServerError(code, msg)
	}
}

func waitFeishuWSDone(ctx context.Context, done <-chan error, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case err := <-done:
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	case <-waitCtx.Done():
		if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("feishu websocket stop timeout after %s", timeout)
		}
		return waitCtx.Err()
	}
}
