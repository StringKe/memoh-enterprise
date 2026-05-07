package connectapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	workspacev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1"
	displaypkg "github.com/memohai/memoh/internal/display"
	"github.com/memohai/memoh/internal/workspace/executorclient"
	scriptassets "github.com/memohai/memoh/scripts"
)

const displayPrepareProgressPrefix = "__MEMOH_DISPLAY_PROGRESS__"

type displayRuntimeProbe struct {
	ToolkitAvailable bool   `json:"toolkit_available"`
	PrepareSupported bool   `json:"prepare_supported"`
	PrepareSystem    string `json:"prepare_system"`
	SessionAvailable bool   `json:"session_available"`
	BrowserAvailable bool   `json:"browser_available"`
	VNCAvailable     bool   `json:"vnc_available"`
}

func (s *ContainerService) GetDisplayInfo(ctx context.Context, req *connect.Request[privatev1.GetDisplayInfoRequest]) (*connect.Response[privatev1.GetDisplayInfoResponse], error) {
	botID, err := s.requireContainerAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	resp := &privatev1.GetDisplayInfoResponse{
		Transport: displaypkg.TransportWebRTC,
		Encoder:   displaypkg.EncoderGStreamer,
	}
	if s.display == nil {
		resp.UnavailableReason = "manager not configured"
		return connect.NewResponse(resp), nil
	}

	status := s.display.Status(ctx, botID)
	resp.Enabled = status.Enabled
	resp.Available = status.Available
	resp.Running = status.Running
	resp.Transport = status.Transport
	resp.Encoder = status.Encoder
	resp.EncoderAvailable = status.EncoderAvailable
	resp.UnavailableReason = status.UnavailableReason

	if !resp.Enabled {
		return connect.NewResponse(resp), nil
	}
	if s.executors == nil {
		resp.UnavailableReason = "workspace executor provider is not configured"
		return connect.NewResponse(resp), nil
	}
	client, err := s.executors.ExecutorClient(ctx, botID)
	if err != nil || client == nil {
		resp.UnavailableReason = "container not reachable"
		return connect.NewResponse(resp), nil
	}
	if probe, ok := probeDisplayRuntime(ctx, client); ok {
		resp.ToolkitAvailable = probe.ToolkitAvailable
		resp.PrepareSupported = probe.PrepareSupported
		resp.PrepareSystem = probe.PrepareSystem
		resp.SessionAvailable = probe.SessionAvailable
		resp.BrowserAvailable = probe.BrowserAvailable
		if !resp.Running && !probe.VNCAvailable {
			resp.UnavailableReason = "display bundle unavailable"
		}
	} else if resp.Available && resp.Running {
		resp.SessionAvailable = true
		resp.BrowserAvailable = true
	}
	return connect.NewResponse(resp), nil
}

func (s *ContainerService) CreateDisplayWebRTCOffer(ctx context.Context, req *connect.Request[privatev1.CreateDisplayWebRTCOfferRequest]) (*connect.Response[privatev1.CreateDisplayWebRTCOfferResponse], error) {
	botID, err := s.requireContainerAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	if s.display == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("display service not configured"))
	}
	answer, err := s.display.Answer(ctx, botID, displaypkg.OfferRequest{
		Type:      req.Msg.GetType(),
		SDP:       req.Msg.GetSdp(),
		SessionID: req.Msg.GetSessionId(),
		NATIPs:    displayNATIPs(ctx, req.Header(), req.Msg.GetCandidateHost()),
	})
	if err != nil {
		return nil, displayConnectError(err)
	}
	return connect.NewResponse(&privatev1.CreateDisplayWebRTCOfferResponse{
		Type:      answer.Type,
		Sdp:       answer.SDP,
		SessionId: answer.SessionID,
	}), nil
}

func (s *ContainerService) ListDisplaySessions(ctx context.Context, req *connect.Request[privatev1.ListDisplaySessionsRequest]) (*connect.Response[privatev1.ListDisplaySessionsResponse], error) {
	botID, err := s.requireContainerAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	if s.display == nil {
		return connect.NewResponse(&privatev1.ListDisplaySessionsResponse{}), nil
	}
	sessions := s.display.ListSessions(botID)
	out := make([]*privatev1.DisplaySession, 0, len(sessions))
	for _, item := range sessions {
		out = append(out, displaySessionToProto(item))
	}
	return connect.NewResponse(&privatev1.ListDisplaySessionsResponse{Sessions: out}), nil
}

func (s *ContainerService) CloseDisplaySession(ctx context.Context, req *connect.Request[privatev1.CloseDisplaySessionRequest]) (*connect.Response[privatev1.CloseDisplaySessionResponse], error) {
	botID, err := s.requireContainerAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	sessionID := strings.TrimSpace(req.Msg.GetSessionId())
	if sessionID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("session_id is required"))
	}
	if s.display == nil || !s.display.CloseSession(botID, sessionID) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("display session not found"))
	}
	return connect.NewResponse(&privatev1.CloseDisplaySessionResponse{}), nil
}

func (s *ContainerService) PrepareDisplay(ctx context.Context, req *connect.Request[privatev1.PrepareDisplayRequest], stream *connect.ServerStream[privatev1.PrepareDisplayResponse]) error {
	botID, err := s.requireContainerAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return err
	}
	send := func(payload *privatev1.PrepareDisplayResponse) error {
		if err := stream.Send(payload); err != nil {
			return err
		}
		return nil
	}
	sendError := func(step, message string) error {
		return send(&privatev1.PrepareDisplayResponse{Type: "error", Step: step, Message: message})
	}

	if s.display == nil {
		return sendError("checking", "display service not configured")
	}
	if s.executors == nil {
		return sendError("checking", "workspace executor provider is not configured")
	}
	if !s.display.Status(ctx, botID).Enabled {
		return sendError("checking", "workspace display is not enabled")
	}
	client, err := s.executors.ExecutorClient(ctx, botID)
	if err != nil || client == nil {
		if err != nil {
			return sendError("checking", "workspace container is not reachable: "+err.Error())
		}
		return sendError("checking", "workspace container is not reachable")
	}
	if err := send(&privatev1.PrepareDisplayResponse{
		Type:    "progress",
		Step:    "checking",
		Message: "Checking display runtime",
		Percent: 5,
	}); err != nil {
		return err
	}

	execStream, err := client.ExecStream(ctx, displayPrepareCommand(), "/", 1200)
	if err != nil {
		return sendError("checking", "start display preparation failed: "+err.Error())
	}
	defer func() { _ = execStream.Close() }()
	_ = execStream.CloseRequest()

	var stdout, stderr lineAccumulator
	var stderrText strings.Builder
	completed := false
	exitCode := int32(0)
	lastStep := "checking"
	for {
		msg, recvErr := execStream.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			return sendError(lastStep, "display preparation stream failed: "+recvErr.Error())
		}
		switch msg.GetKind() {
		case workspacev1.ExecResponse_KIND_STDOUT:
			for _, line := range stdout.Append(msg.GetData()) {
				event, ok := parseDisplayPrepareEvent(line)
				if !ok {
					continue
				}
				if event.GetStep() != "" {
					lastStep = event.GetStep()
				}
				if err := send(event); err != nil {
					return err
				}
				if event.GetType() == "complete" {
					completed = true
				}
			}
		case workspacev1.ExecResponse_KIND_STDERR:
			for _, line := range stderr.Append(msg.GetData()) {
				appendLimitedLine(&stderrText, line)
			}
		case workspacev1.ExecResponse_KIND_EXIT:
			exitCode = msg.GetExitCode()
		case workspacev1.ExecResponse_KIND_ERROR:
			return sendError(lastStep, strings.TrimSpace(msg.GetErrorMessage()))
		}
	}
	for _, line := range stdout.Flush() {
		event, ok := parseDisplayPrepareEvent(line)
		if !ok {
			continue
		}
		if event.GetStep() != "" {
			lastStep = event.GetStep()
		}
		if err := send(event); err != nil {
			return err
		}
		if event.GetType() == "complete" {
			completed = true
		}
	}
	for _, line := range stderr.Flush() {
		appendLimitedLine(&stderrText, line)
	}
	if exitCode != 0 && !completed {
		message := strings.TrimSpace(stderrText.String())
		if message == "" {
			message = "display preparation failed"
		}
		return sendError(lastStep, message)
	}
	if !completed {
		return send(&privatev1.PrepareDisplayResponse{
			Type:    "complete",
			Step:    "complete",
			Message: "Display is ready",
			Percent: 100,
		})
	}
	return nil
}

func displayConnectError(err error) error {
	switch {
	case errors.Is(err, displaypkg.ErrDisplayDisabled):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, displaypkg.ErrDisplayUnavailable), errors.Is(err, displaypkg.ErrEncoderUnavailable):
		return connect.NewError(connect.CodeUnavailable, err)
	default:
		return connectError(err)
	}
}

func displaySessionToProto(info displaypkg.SessionInfo) *privatev1.DisplaySession {
	return &privatev1.DisplaySession{
		Id:        info.ID,
		Codec:     info.Codec,
		State:     info.State,
		CreatedAt: timestamppb.New(info.CreatedAt),
	}
}

type lineAccumulator struct {
	partial string
}

func (b *lineAccumulator) Append(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	text := b.partial + string(data)
	parts := strings.Split(text, "\n")
	b.partial = parts[len(parts)-1]
	lines := parts[:len(parts)-1]
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	return lines
}

func (b *lineAccumulator) Flush() []string {
	if b.partial == "" {
		return nil
	}
	line := strings.TrimRight(b.partial, "\r")
	b.partial = ""
	return []string{line}
}

func parseDisplayPrepareEvent(line string) (*privatev1.PrepareDisplayResponse, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, displayPrepareProgressPrefix) {
		return nil, false
	}
	var event privatev1.PrepareDisplayResponse
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, displayPrepareProgressPrefix)), &event); err != nil {
		return nil, false
	}
	return &event, true
}

func appendLimitedLine(builder *strings.Builder, line string) {
	line = strings.TrimSpace(line)
	if line == "" || builder.Len() > 6000 {
		return
	}
	if builder.Len() > 0 {
		builder.WriteByte('\n')
	}
	builder.WriteString(line)
}

func displayPrepareCommand() string {
	return `cat >/tmp/memoh-display-install.sh <<'MEMOH_DISPLAY_INSTALL'
` + strings.TrimRight(displayPrepareInstallScript(), "\n") + `
MEMOH_DISPLAY_INSTALL
chmod 0755 /tmp/memoh-display-install.sh
` + displayPrepareMainCommand
}

func displayPrepareInstallScript() string {
	if data, err := os.ReadFile("scripts/display-install.sh"); err == nil {
		return string(data)
	}
	return scriptassets.DisplayInstall
}

func probeDisplayRuntime(ctx context.Context, client *executorclient.Client) (displayRuntimeProbe, bool) {
	var probe displayRuntimeProbe
	if client == nil {
		return probe, false
	}
	for attempt := 0; attempt < 3; attempt++ {
		result, err := client.Exec(ctx, displayRuntimeProbeCommand, "/", 10)
		if err == nil && result != nil && result.ExitCode == 0 {
			if err := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &probe); err == nil {
				return probe, true
			}
		}
		if attempt == 2 {
			break
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return probe, false
		case <-timer.C:
		}
	}
	return probe, false
}

func displayNATIPs(ctx context.Context, headers http.Header, candidateHost string) []string {
	hosts := []string{
		candidateHost,
		firstHeaderValue(headers.Get("X-Forwarded-Host")),
	}
	seen := make(map[string]struct{})
	var ips []string
	for _, host := range hosts {
		for _, ip := range resolveDisplayHostIPs(ctx, host) {
			if _, ok := seen[ip]; ok {
				continue
			}
			seen[ip] = struct{}{}
			ips = append(ips, ip)
		}
	}
	return ips
}

func firstHeaderValue(value string) string {
	value = strings.TrimSpace(value)
	if idx := strings.IndexByte(value, ','); idx >= 0 {
		return strings.TrimSpace(value[:idx])
	}
	return value
}

func resolveDisplayHostIPs(ctx context.Context, value string) []string {
	host := strings.TrimSpace(value)
	if host == "" {
		return nil
	}
	if strings.HasPrefix(host, "[") {
		if end := strings.Index(host, "]"); end >= 0 {
			host = host[1:end]
		}
	} else if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	} else if strings.Count(host, ":") == 0 {
		if idx := strings.LastIndexByte(host, ':'); idx >= 0 {
			host = host[:idx]
		}
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		return []string{ip.String()}
	}
	resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(resolved))
	for _, ip := range resolved {
		if ip.IP == nil {
			continue
		}
		out = append(out, ip.IP.String())
	}
	return out
}

const displayPrepareMainCommand = `cat >/tmp/memoh-display-prepare.sh <<'MEMOH_DISPLAY_PREPARE'
#!/bin/sh
set -eu

prefix='__MEMOH_DISPLAY_PROGRESS__'
progress() {
  percent="$1"
  step="$2"
  shift 2
  message="$*"
  printf '%s{"type":"progress","percent":%s,"step":"%s","message":"%s"}\n' "$prefix" "$percent" "$step" "$message"
}
complete() {
  printf '%s{"type":"complete","percent":100,"step":"complete","message":"Display is ready"}\n' "$prefix"
}
has_cmd() {
  command -v "$1" >/dev/null 2>&1
}
find_xvnc() {
  for candidate in /opt/memoh/toolkit/display/bin/Xvnc /usr/bin/Xvnc /usr/local/bin/Xvnc Xvnc; do
    if echo "$candidate" | grep -q /; then
      [ -x "$candidate" ] && { printf '%s\n' "$candidate"; return 0; }
    elif has_cmd "$candidate"; then
      command -v "$candidate"
      return 0
    fi
  done
  return 1
}
find_browser() {
  for candidate in google-chrome-stable google-chrome chromium chromium-browser; do
    if has_cmd "$candidate"; then
      command -v "$candidate"
      return 0
    fi
  done
  return 1
}
has_display_session() {
  has_cmd startxfce4 || has_cmd xfce4-session || has_cmd xfwm4 || [ -x /opt/memoh/toolkit/display/bin/twm ]
}
has_toolkit() {
  [ -x /opt/memoh/toolkit/display/bin/Xvnc ] || [ -x /opt/memoh/toolkit/display/bin/twm ]
}
needs_install() {
  find_xvnc >/dev/null 2>&1 && find_browser >/dev/null 2>&1 && has_display_session
}
os_id() {
  if [ -r /etc/os-release ]; then
    . /etc/os-release
    printf '%s\n' "${ID:-unknown}"
    return
  fi
  printf unknown
}
os_like() {
  if [ -r /etc/os-release ]; then
    . /etc/os-release
    printf '%s %s\n' "${ID:-}" "${ID_LIKE:-}"
    return
  fi
  printf unknown
}
is_debian_like() {
  case " $(os_like) " in
    *" debian "*|*" ubuntu "*) return 0 ;;
    *) return 1 ;;
  esac
}
is_alpine() {
  case " $(os_like) " in
    *" alpine "*) return 0 ;;
    *) return 1 ;;
  esac
}
RFB_SOCKET=/run/memoh/display.rfb.sock
X_SOCKET=/tmp/.X11-unix/X99
X_LOCK=/tmp/.X99-lock
xvnc_pids() {
  for proc_dir in /proc/[0-9]*; do
    [ -d "$proc_dir" ] || continue
    pid="${proc_dir#/proc/}"
    cmdline="$(tr '\000' '\n' <"$proc_dir/cmdline" 2>/dev/null || true)"
    printf '%s\n' "$cmdline" | grep -Eq '(^|/)Xvnc$' || continue
    printf '%s\n' "$cmdline" | grep -Fxq ':99' || continue
    printf '%s\n' "$pid"
  done
  return 0
}
xvnc_running() {
  [ -n "$(xvnc_pids)" ]
}
browser_pids() {
  for proc_dir in /proc/[0-9]*; do
    [ -d "$proc_dir" ] || continue
    pid="${proc_dir#/proc/}"
    cmdline="$(tr '\000' '\n' <"$proc_dir/cmdline" 2>/dev/null || true)"
    printf '%s\n' "$cmdline" | grep -Eq '(^|/)(google-chrome-stable|google-chrome|chromium|chromium-browser|chrome)$' || continue
    printf '%s\n' "$pid"
  done
  return 0
}
browser_cdp_running() {
  for proc_dir in /proc/[0-9]*; do
    [ -d "$proc_dir" ] || continue
    cmdline="$(tr '\000' '\n' <"$proc_dir/cmdline" 2>/dev/null || true)"
    printf '%s\n' "$cmdline" | grep -Eq '(^|/)(google-chrome-stable|google-chrome|chromium|chromium-browser|chrome)$' || continue
    printf '%s\n' "$cmdline" | grep -Eq '^--type=' && continue
    printf '%s\n' "$cmdline" | grep -Fq -- '--remote-debugging-port=9222' && return 0
  done
  return 1
}
cleanup_browser_profile() {
  [ -n "$(browser_pids)" ] && return 0
  rm -f /tmp/memoh-display-browser/SingletonLock /tmp/memoh-display-browser/SingletonSocket /tmp/memoh-display-browser/SingletonCookie
}
stop_xvnc() {
  pids="$(xvnc_pids)"
  [ -n "$pids" ] || return 0
  for pid in $pids; do
    kill "$pid" 2>/dev/null || true
  done
  sleep 1
  pids="$(xvnc_pids)"
  for pid in $pids; do
    kill -9 "$pid" 2>/dev/null || true
  done
}
stop_browsers() {
  pids="$(browser_pids)"
  [ -n "$pids" ] || return 0
  for pid in $pids; do
    kill "$pid" 2>/dev/null || true
  done
  sleep 1
  pids="$(browser_pids)"
  for pid in $pids; do
    kill -9 "$pid" 2>/dev/null || true
  done
}
display_socket_ready() {
  xvnc_running && [ -S "$RFB_SOCKET" ] && [ -S "$X_SOCKET" ]
}
display_ready() {
  display_socket_ready && find_browser >/dev/null 2>&1 && has_display_session
}

. /tmp/memoh-display-install.sh

prepare_lock=/tmp/memoh-display-prepare.lock
if mkdir "$prepare_lock" 2>/dev/null; then
  trap 'rmdir "$prepare_lock" 2>/dev/null || true' EXIT INT TERM
else
  progress 12 checking "Waiting for another display preparation"
  wait_i=0
  while [ -d "$prepare_lock" ] && [ "$wait_i" -lt 180 ]; do
    if display_ready; then
      complete
      exit 0
    fi
    sleep 1
    wait_i=$((wait_i + 1))
  done
  if display_ready; then
    complete
    exit 0
  fi
  echo "Another display preparation is still running." >&2
  exit 1
fi

progress 10 checking "Checking display toolkit"
if ! has_toolkit; then
  progress 14 toolkit "Workspace display toolkit is not installed"
fi

if needs_install; then
  progress 18 checking "Display packages already installed"
else
  if is_debian_like; then
    install_debian
  elif is_alpine; then
    install_alpine
  else
    echo "Unsupported workspace OS: $(os_id). Install the Memoh workspace toolkit, or use a Debian/Ubuntu/Alpine image for automatic display preparation." >&2
    exit 1
  fi
fi

XVNC="$(find_xvnc || true)"
BROWSER="$(find_browser || true)"
[ -n "$XVNC" ] || { echo "Xvnc is still unavailable after installation. Install the Memoh workspace toolkit or a TigerVNC package." >&2; exit 1; }
[ -n "$BROWSER" ] || { echo "Chrome or Chromium is still unavailable after installation." >&2; exit 1; }

export DISPLAY=:99
mkdir -p /run/memoh /tmp/.X11-unix
chmod 1777 /tmp/.X11-unix 2>/dev/null || true

wait_for_socket() {
  path="$1"
  seconds="$2"
  i=0
  while [ "$i" -lt "$seconds" ]; do
    [ -S "$path" ] && return 0
    sleep 1
    i=$((i + 1))
  done
  return 1
}

cleanup_stale_display() {
  xvnc_running && return 0
  rm -f "$RFB_SOCKET" "$X_SOCKET" "$X_LOCK"
}

if ! display_socket_ready; then
  progress 78 starting "Starting VNC display"
  if xvnc_running; then
    wait_for_socket "$RFB_SOCKET" 10 || true
  fi
  if ! display_socket_ready; then
    stop_xvnc
    cleanup_stale_display
    nohup "$XVNC" :99 -geometry 1280x800 -depth 24 -SecurityTypes None -rfbunixpath "$RFB_SOCKET" -rfbunixmode 0660 -rfbport 0 >/tmp/memoh-xvnc.log 2>&1 &
    wait_i=0
    while [ "$wait_i" -lt 25 ]; do
      display_socket_ready && break
      sleep 1
      wait_i=$((wait_i + 1))
    done
    display_socket_ready || { cat /tmp/memoh-xvnc.log >&2 2>/dev/null || true; exit 1; }
  fi
fi

progress 88 session "Starting display session"
run_quick() {
  if command -v timeout >/dev/null 2>&1; then
    timeout 5 "$@" >/dev/null 2>&1 || true
  else
    "$@" >/dev/null 2>&1 &
  fi
}
if command -v fc-cache >/dev/null 2>&1; then
  nohup fc-cache -f >/tmp/memoh-fc-cache.log 2>&1 &
fi
if [ -S "$X_SOCKET" ]; then
  if command -v xsetroot >/dev/null 2>&1; then
    run_quick xsetroot -solid '#315f7d'
  elif [ -x /opt/memoh/toolkit/display/bin/xsetroot ]; then
    run_quick /opt/memoh/toolkit/display/bin/xsetroot -solid '#315f7d'
  fi
fi
if ! ps -ef 2>/dev/null | grep -E 'xfce4-session|xfwm4|twm' | grep -v grep >/dev/null 2>&1; then
  if has_cmd startxfce4; then
    nohup startxfce4 >/tmp/memoh-xfce.log 2>&1 &
  elif has_cmd xfce4-session; then
    nohup xfce4-session >/tmp/memoh-xfce.log 2>&1 &
  elif has_cmd xfwm4; then
    nohup xfwm4 >/tmp/memoh-xfwm4.log 2>&1 &
  elif [ -x /opt/memoh/toolkit/display/bin/twm ]; then
    nohup /opt/memoh/toolkit/display/bin/twm >/tmp/memoh-twm.log 2>&1 &
  fi
fi

progress 94 browser "Launching browser"
if ! browser_cdp_running; then
  if [ -n "$(browser_pids)" ]; then
    stop_browsers
  fi
  cleanup_browser_profile
  nohup "$BROWSER" --no-sandbox --disable-dev-shm-usage --disable-gpu --no-first-run --no-default-browser-check --remote-debugging-address=127.0.0.1 --remote-debugging-port=9222 --remote-allow-origins='*' --user-data-dir=/tmp/memoh-display-browser about:blank >/tmp/memoh-browser.log 2>&1 &
fi

complete
exit 0
MEMOH_DISPLAY_PREPARE
/bin/sh /tmp/memoh-display-prepare.sh`

const displayRuntimeProbeCommand = `has_cmd() { command -v "$1" >/dev/null 2>&1; }
has_exec() { [ -x "$1" ]; }
has_process() { ps -ef 2>/dev/null | grep -E "$1" | grep -v grep >/dev/null 2>&1; }
json_bool() { if "$@"; then printf true; else printf false; fi; }
os_id=unknown
os_like=
if [ -r /etc/os-release ]; then
  . /etc/os-release
  os_id="${ID:-unknown}"
  os_like="${ID:-} ${ID_LIKE:-}"
fi
has_toolkit() {
  has_exec /opt/memoh/toolkit/display/bin/Xvnc ||
    has_exec /opt/memoh/toolkit/display/bin/twm ||
    has_exec /opt/memoh/toolkit/display/root/usr/bin/Xvnc ||
    has_exec /opt/memoh/toolkit/display/root/usr/bin/twm
}
has_prepare() {
  case " $os_like " in
    *" debian "*|*" ubuntu "*|*" alpine "*) return 0 ;;
    *) return 1 ;;
  esac
}
has_vnc() {
  has_cmd Xvnc ||
    has_exec /opt/memoh/toolkit/display/bin/Xvnc ||
    has_exec /opt/memoh/toolkit/display/root/usr/bin/Xvnc ||
    has_exec /usr/bin/Xvnc ||
    has_exec /usr/local/bin/Xvnc
}
has_display_session() {
  has_cmd startxfce4 ||
    has_cmd xfce4-session ||
    has_cmd xfwm4 ||
    has_exec /opt/memoh/toolkit/display/bin/twm ||
    has_exec /opt/memoh/toolkit/display/root/usr/bin/twm ||
    has_process 'xfce4-session|xfwm4|twm'
}
has_browser() {
  has_cmd google-chrome-stable ||
    has_cmd google-chrome ||
    has_cmd chromium ||
    has_cmd chromium-browser ||
    has_process 'google-chrome|chromium'
}
printf '{"toolkit_available":%s,"prepare_supported":%s,"prepare_system":"%s","session_available":%s,"browser_available":%s,"vnc_available":%s}\n' \
  "$(json_bool has_toolkit)" \
  "$(json_bool has_prepare)" \
  "$os_id" \
  "$(json_bool has_display_session)" \
  "$(json_bool has_browser)" \
  "$(json_bool has_vnc)"`
