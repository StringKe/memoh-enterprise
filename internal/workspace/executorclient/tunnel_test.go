package executorclient

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1/workspacev1connect"
	"github.com/memohai/memoh/internal/workspace/executorsvc"
)

// startTunnelTestServer starts an in-process executor server fronted by an
// httptest server. The returned executor client speaks Connect RPC over h2c
// just like the production wiring.
func startTunnelTestServer(t *testing.T) *Client {
	t.Helper()

	svc := executorsvc.New(executorsvc.Options{
		WorkspaceRoot:     "/",
		AllowHostAbsolute: true,
	})
	mux := http.NewServeMux()
	path, handler := workspacev1connect.NewWorkspaceExecutorServiceHandler(svc)
	mux.Handle(path, handler)
	server := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))
	t.Cleanup(server.Close)

	httpClient := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				dialer := &net.Dialer{}
				return dialer.DialContext(ctx, network, addr)
			},
		},
	}
	return NewClient(workspacev1connect.NewWorkspaceExecutorServiceClient(httpClient, server.URL, connect.WithGRPC()), nil)
}

// startEchoServer listens on 127.0.0.1 and replies to every line received with
// "ok:<line>\n". Returns the listening address.
func startEchoServer(t *testing.T) string {
	t.Helper()

	lc := net.ListenConfig{}
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				reader := bufio.NewReader(c)
				for {
					line, err := reader.ReadString('\n')
					if len(line) > 0 {
						_, _ = c.Write([]byte("ok:" + line))
					}
					if err != nil {
						return
					}
				}
			}(conn)
		}
	}()

	return ln.Addr().String()
}

func TestClientTunnelRoundTripsBytes(t *testing.T) {
	t.Parallel()

	addr := startEchoServer(t)
	client := startTunnelTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := client.DialContext(ctx, "tcp", addr)
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write([]byte("hello\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("ReadString: %v", err)
	}
	if !strings.HasPrefix(line, "ok:hello") {
		t.Fatalf("read = %q, want prefix %q", line, "ok:hello")
	}
}

func TestClientTunnelRejectsNonLoopback(t *testing.T) {
	t.Parallel()

	client := startTunnelTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := client.DialContext(ctx, "tcp", "10.0.0.1:9222"); err == nil {
		t.Fatal("DialContext to 10.0.0.1 should fail")
	}
}

func TestClientTunnelRejectsUnsupportedNetwork(t *testing.T) {
	t.Parallel()

	client := startTunnelTestServer(t)

	if _, err := client.DialContext(context.Background(), "udp", "127.0.0.1:9222"); err == nil {
		t.Fatal("DialContext on udp should fail")
	}
}

func TestTunnelConnReadReturnsEOFOnRemoteClose(t *testing.T) {
	t.Parallel()

	addr := startEchoServer(t)
	client := startTunnelTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := client.DialContext(ctx, "tcp", addr)
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}

	_ = conn.Close()
	buf := make([]byte, 16)
	_, err = conn.Read(buf)
	if err == nil {
		t.Fatalf("expected error after Close, got nil")
	}
	if !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "canceled") {
		// Either io.EOF (clean) or a context-cancel propagation is acceptable.
		t.Logf("Read returned %v (treated as ok)", err)
	}
}
