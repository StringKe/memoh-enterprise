package feishu

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestFeishuWSClientCloseClosesWebsocket(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	var endpointRequests atomic.Int64
	connected := make(chan struct{})
	socketClosed := make(chan struct{})
	var closeConnected sync.Once
	var closeSocketClosed sync.Once
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?device_id=device-1&service_id=1"
	mux.HandleFunc("/callback/ws/endpoint", func(w http.ResponseWriter, r *http.Request) {
		endpointRequests.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("unexpected endpoint method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"code":0,"data":{"URL":%q,"ClientConfig":{"ReconnectCount":-1,"ReconnectInterval":1,"ReconnectNonce":0,"PingInterval":1}}}`, wsURL)
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		closeConnected.Do(func() { close(connected) })
		defer closeSocketClosed.Do(func() { close(socketClosed) })
		defer func() { _ = conn.Close() }()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})

	client := newFeishuWSClient(testWSConfig(), nil, nil, withFeishuWSBaseURL(server.URL))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- client.Run(ctx)
	}()

	select {
	case <-connected:
	case <-time.After(time.Second):
		t.Fatal("websocket was not connected")
	}
	cancel()
	if err := client.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	select {
	case <-socketClosed:
	case <-time.After(time.Second):
		t.Fatal("server did not observe websocket close")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned error after cancel: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("client run did not exit")
	}
	if got := endpointRequests.Load(); got != 1 {
		t.Fatalf("unexpected endpoint requests: %d", got)
	}
}

func TestFeishuWSClientDoesNotReconnectAfterContextCancel(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	var endpointRequests atomic.Int64
	firstSocketClosed := make(chan struct{})
	var closeFirstSocket sync.Once
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?device_id=device-1&service_id=1"
	mux.HandleFunc("/callback/ws/endpoint", func(w http.ResponseWriter, _ *http.Request) {
		endpointRequests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"code":0,"data":{"URL":%q,"ClientConfig":{"ReconnectCount":-1,"ReconnectInterval":1,"ReconnectNonce":0,"PingInterval":1}}}`, wsURL)
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		_ = conn.Close()
		closeFirstSocket.Do(func() { close(firstSocketClosed) })
	})

	client := newFeishuWSClient(testWSConfig(), nil, nil, withFeishuWSBaseURL(server.URL))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- client.Run(ctx)
	}()

	select {
	case <-firstSocketClosed:
	case <-time.After(time.Second):
		t.Fatal("websocket was not closed by test server")
	}
	cancel()
	if err := client.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned error after cancel: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("client run did not exit")
	}
	time.Sleep(100 * time.Millisecond)
	if got := endpointRequests.Load(); got != 1 {
		t.Fatalf("client reconnected after cancel; endpoint requests: %d", got)
	}
}

func testWSConfig() Config {
	return Config{
		AppID:       "app",
		AppSecret:   "secret",
		Region:      regionFeishu,
		InboundMode: inboundModeWebsocket,
	}
}
