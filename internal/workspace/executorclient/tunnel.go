package executorclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"connectrpc.com/connect"

	workspacev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1"
)

// DialContext opens a Tunnel stream to the workspace executor and returns a
// net.Conn that proxies bytes through it. Only loopback addresses inside the
// workspace are accepted by the server.
func (c *Client) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if c == nil {
		return nil, errors.New("executor client is not configured")
	}
	if network != "tcp" {
		return nil, fmt.Errorf("unsupported tunnel network %q", network)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stream := c.svc.Tunnel(streamCtx)
	if err := stream.Send(&workspacev1.TunnelFrame{
		Frame: &workspacev1.TunnelFrame_Open{Open: &workspacev1.TunnelOpen{Address: address}},
	}); err != nil {
		cancel()
		_ = stream.CloseRequest()
		return nil, mapError(err)
	}
	conn := &tunnelConn{
		stream: stream,
		cancel: cancel,
		local:  tunnelAddr("memoh-executor"),
		remote: tunnelAddr(address),
	}
	if err := conn.waitOpen(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

type tunnelConn struct {
	stream *connect.BidiStreamForClient[workspacev1.TunnelFrame, workspacev1.TunnelFrame]
	cancel context.CancelFunc
	local  net.Addr
	remote net.Addr

	readMu   sync.Mutex
	writeMu  sync.Mutex
	closeMu  sync.Once
	pending  []byte
	readDone bool
	readErr  error

	deadline      time.Time
	readDeadline  time.Time
	writeDeadline time.Time
}

type tunnelAddr string

func (tunnelAddr) Network() string  { return "executor-tunnel" }
func (a tunnelAddr) String() string { return string(a) }

func (c *tunnelConn) waitOpen(ctx context.Context) error {
	type recvResult struct {
		frame *workspacev1.TunnelFrame
		err   error
	}
	resultCh := make(chan recvResult, 1)
	go func() {
		frame, err := c.stream.Receive()
		resultCh <- recvResult{frame: frame, err: err}
	}()
	select {
	case result := <-resultCh:
		if result.err != nil {
			return result.err
		}
		switch f := result.frame.GetFrame().(type) {
		case *workspacev1.TunnelFrame_Open:
			return nil
		case *workspacev1.TunnelFrame_Close:
			msg := f.Close.GetError()
			if msg == "" {
				msg = "tunnel rejected by server"
			}
			return errors.New(msg)
		default:
			return errors.New("tunnel handshake response missing open ack")
		}
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *tunnelConn) Read(p []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if len(c.pending) > 0 {
		n := copy(p, c.pending)
		c.pending = c.pending[n:]
		return n, nil
	}
	if c.readDone {
		return 0, c.readErr
	}
	for {
		frame, err := c.stream.Receive()
		if err != nil {
			c.readDone = true
			c.readErr = err
			if errors.Is(err, io.EOF) {
				return 0, io.EOF
			}
			return 0, err
		}
		switch f := frame.GetFrame().(type) {
		case *workspacev1.TunnelFrame_Data:
			data := f.Data.GetData()
			if len(data) == 0 {
				continue
			}
			n := copy(p, data)
			if n < len(data) {
				c.pending = append(c.pending, data[n:]...)
			}
			return n, nil
		case *workspacev1.TunnelFrame_Close:
			c.readDone = true
			msg := f.Close.GetError()
			if msg != "" {
				c.readErr = errors.New(msg)
				return 0, c.readErr
			}
			c.readErr = io.EOF
			return 0, io.EOF
		case *workspacev1.TunnelFrame_Open:
			// Stray opens after handshake are ignored.
		}
	}
}

func (c *tunnelConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	out := make([]byte, len(p))
	copy(out, p)
	if err := c.stream.Send(&workspacev1.TunnelFrame{
		Frame: &workspacev1.TunnelFrame_Data{Data: &workspacev1.TunnelData{Data: out}},
	}); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *tunnelConn) Close() error {
	c.closeMu.Do(func() {
		c.writeMu.Lock()
		_ = c.stream.Send(&workspacev1.TunnelFrame{
			Frame: &workspacev1.TunnelFrame_Close{Close: &workspacev1.TunnelClose{}},
		})
		_ = c.stream.CloseRequest()
		c.writeMu.Unlock()
		_ = c.stream.CloseResponse()
		if c.cancel != nil {
			c.cancel()
		}
	})
	return nil
}

func (c *tunnelConn) LocalAddr() net.Addr  { return c.local }
func (c *tunnelConn) RemoteAddr() net.Addr { return c.remote }

func (c *tunnelConn) SetDeadline(t time.Time) error {
	c.deadline = t
	c.readDeadline = t
	c.writeDeadline = t
	return nil
}

func (c *tunnelConn) SetReadDeadline(t time.Time) error {
	c.readDeadline = t
	return nil
}

func (c *tunnelConn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline = t
	return nil
}
