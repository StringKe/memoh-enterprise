package connectapi

import (
	"context"
	"net/http"
	"testing"
)

func TestParseDisplayPrepareEvent(t *testing.T) {
	line := displayPrepareProgressPrefix + `{"type":"progress","step":"session","message":"Starting display session","percent":88}`

	event, ok := parseDisplayPrepareEvent(line)
	if !ok {
		t.Fatal("expected display prepare event")
	}
	if event.GetType() != "progress" || event.GetStep() != "session" || event.GetPercent() != 88 {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestParseDisplayPrepareEventRejectsNonProgressLine(t *testing.T) {
	if event, ok := parseDisplayPrepareEvent("ordinary stdout"); ok || event != nil {
		t.Fatalf("expected ordinary stdout to be ignored, got %+v", event)
	}
}

func TestLineAccumulatorKeepsPartialLine(t *testing.T) {
	var acc lineAccumulator

	lines := acc.Append([]byte("one\ntw"))
	if len(lines) != 1 || lines[0] != "one" {
		t.Fatalf("unexpected initial lines: %#v", lines)
	}
	lines = acc.Append([]byte("o\r\nthree"))
	if len(lines) != 1 || lines[0] != "two" {
		t.Fatalf("unexpected second lines: %#v", lines)
	}
	lines = acc.Flush()
	if len(lines) != 1 || lines[0] != "three" {
		t.Fatalf("unexpected flushed lines: %#v", lines)
	}
}

func TestDisplayNATIPsUsesCandidateAndForwardedHosts(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Forwarded-Host", "10.0.0.2:8443, 10.0.0.3")

	ips := displayNATIPs(context.Background(), headers, "192.168.1.10")
	if len(ips) != 2 || ips[0] != "192.168.1.10" || ips[1] != "10.0.0.2" {
		t.Fatalf("unexpected NAT IPs: %#v", ips)
	}
}
