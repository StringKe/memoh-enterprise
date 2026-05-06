package executorsvc

import (
	"context"
	"io"
	"testing"

	"google.golang.org/protobuf/proto"

	workspacev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1"
)

type cancelOnStdoutExecStream struct {
	ctx    context.Context
	cancel context.CancelFunc

	outputs  []*workspacev1.ExecResponse
	canceled bool
}

func newCancelOnStdoutExecStream() *cancelOnStdoutExecStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &cancelOnStdoutExecStream{ctx: ctx, cancel: cancel}
}

func (s *cancelOnStdoutExecStream) Send(msg *workspacev1.ExecResponse) error {
	clone := proto.Clone(msg).(*workspacev1.ExecResponse)
	if len(msg.GetData()) > 0 {
		clone.Data = append([]byte(nil), msg.GetData()...)
	}
	s.outputs = append(s.outputs, clone)
	if !s.canceled && msg.GetKind() == workspacev1.ExecResponse_KIND_STDOUT && len(msg.GetData()) > 0 {
		s.canceled = true
		s.cancel()
	}
	return nil
}

func (s *cancelOnStdoutExecStream) Receive() (*workspacev1.ExecRequest, error) {
	<-s.ctx.Done()
	return nil, io.EOF
}

func (s *cancelOnStdoutExecStream) Context() context.Context { return s.ctx }

func TestExecPipePreservesExitCodeAcrossStreamCancellation(t *testing.T) {
	stream := newCancelOnStdoutExecStream()
	srv := New(Options{DefaultWorkDir: "/tmp", AllowHostAbsolute: true})

	err := srv.execPipe(context.Background(), stream, &workspacev1.ExecStart{
		Command:        "printf ok; sleep 0.2",
		WorkDir:        "/tmp",
		TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("execPipe returned error: %v", err)
	}

	var stdout string
	var exitCode int32 = -999
	for _, output := range stream.outputs {
		switch output.GetKind() {
		case workspacev1.ExecResponse_KIND_STDOUT:
			stdout += string(output.GetData())
		case workspacev1.ExecResponse_KIND_EXIT:
			exitCode = output.GetExitCode()
		}
	}

	if stdout != "ok" {
		t.Fatalf("stdout = %q, want %q", stdout, "ok")
	}
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
}
