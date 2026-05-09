package executorsvc

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"time"

	"connectrpc.com/connect"

	workspacev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1"
)

const (
	tunnelDialTimeout = 5 * time.Second
	tunnelReadBuffer  = 32 * 1024
)

// Tunnel proxies a raw bytes stream between the caller and a TCP target inside
// the workspace. The caller opens the tunnel by sending a TunnelOpen frame
// with the destination address; the server replies with TunnelOpen on success
// or TunnelClose carrying the dial error. Subsequent frames carry TunnelData
// in either direction; either side terminates by sending TunnelClose.
//
// Address validation is intentionally narrow: only loopback hosts are allowed
// so the tunnel cannot be used to escape the workspace network namespace.
func (*Server) Tunnel(ctx context.Context, stream *connect.BidiStream[workspacev1.TunnelFrame, workspacev1.TunnelFrame]) error {
	first, err := stream.Receive()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	open := first.GetOpen()
	if open == nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("tunnel first frame must be TunnelOpen"))
	}
	address := strings.TrimSpace(open.GetAddress())
	if err := validateTunnelAddress(address); err != nil {
		return sendTunnelClose(stream, err)
	}

	dialCtx, cancel := context.WithTimeout(ctx, tunnelDialTimeout)
	defer cancel()
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(dialCtx, "tcp", address)
	if err != nil {
		return sendTunnelClose(stream, err)
	}
	defer func() { _ = conn.Close() }()

	if err := stream.Send(&workspacev1.TunnelFrame{
		Frame: &workspacev1.TunnelFrame_Open{Open: &workspacev1.TunnelOpen{Address: address}},
	}); err != nil {
		return err
	}

	errCh := make(chan error, 2)
	// upstream: stream -> conn
	go func() {
		errCh <- copyStreamToConn(stream, conn)
	}()
	// downstream: conn -> stream
	go func() {
		errCh <- copyConnToStream(conn, stream)
	}()

	first2 := <-errCh
	_ = conn.Close()
	<-errCh

	if first2 != nil && !errors.Is(first2, io.EOF) && !errors.Is(first2, net.ErrClosed) {
		return first2
	}
	return nil
}

func copyStreamToConn(stream *connect.BidiStream[workspacev1.TunnelFrame, workspacev1.TunnelFrame], conn net.Conn) error {
	for {
		frame, err := stream.Receive()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		switch f := frame.GetFrame().(type) {
		case *workspacev1.TunnelFrame_Data:
			if _, err := conn.Write(f.Data.GetData()); err != nil {
				return err
			}
		case *workspacev1.TunnelFrame_Close:
			return nil
		case *workspacev1.TunnelFrame_Open:
			// Ignore stray opens after the handshake.
		}
	}
}

func copyConnToStream(conn net.Conn, stream *connect.BidiStream[workspacev1.TunnelFrame, workspacev1.TunnelFrame]) error {
	buf := make([]byte, tunnelReadBuffer)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			out := make([]byte, n)
			copy(out, buf[:n])
			if sendErr := stream.Send(&workspacev1.TunnelFrame{
				Frame: &workspacev1.TunnelFrame_Data{Data: &workspacev1.TunnelData{Data: out}},
			}); sendErr != nil {
				return sendErr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				_ = stream.Send(&workspacev1.TunnelFrame{
					Frame: &workspacev1.TunnelFrame_Close{Close: &workspacev1.TunnelClose{}},
				})
				return nil
			}
			return err
		}
	}
}

func sendTunnelClose(stream *connect.BidiStream[workspacev1.TunnelFrame, workspacev1.TunnelFrame], cause error) error {
	msg := ""
	if cause != nil {
		msg = cause.Error()
	}
	if err := stream.Send(&workspacev1.TunnelFrame{
		Frame: &workspacev1.TunnelFrame_Close{Close: &workspacev1.TunnelClose{Error: msg}},
	}); err != nil {
		return err
	}
	return connect.NewError(connect.CodeFailedPrecondition, cause)
}

// validateTunnelAddress restricts tunnel targets to loopback host:port pairs
// so the workspace tunnel cannot reach arbitrary hosts on the workspace
// network. The agent only needs CDP (Chromium devtools) which always binds to
// 127.0.0.1 inside the workspace container.
func validateTunnelAddress(address string) error {
	if address == "" {
		return errors.New("tunnel address is required")
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if port == "" {
		return errors.New("tunnel address must include a port")
	}
	switch strings.TrimSpace(host) {
	case "127.0.0.1", "::1", "localhost":
		return nil
	}
	return errors.New("tunnel only allows loopback hosts (127.0.0.1, ::1, localhost)")
}
