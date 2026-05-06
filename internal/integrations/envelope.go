package integrations

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/encoding/protojson"

	integrationv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/integration/v1"
)

const (
	wsProtocolVersion = "2026-05-05"
	wsWriteTimeout    = 10 * time.Second
)

func readEnvelope(conn *websocket.Conn, envelope *integrationv1.Envelope) error {
	messageType, payload, err := conn.ReadMessage()
	if err != nil {
		return err
	}
	if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
		return errors.New("unsupported websocket message type")
	}
	return protojson.Unmarshal(payload, envelope)
}

func marshalEnvelope(envelope *integrationv1.Envelope) ([]byte, error) {
	return protojson.MarshalOptions{EmitUnpopulated: false}.Marshal(envelope)
}

func responseEnvelope(correlationID string, payload any) *integrationv1.Envelope {
	envelope := &integrationv1.Envelope{
		Version:       wsProtocolVersion,
		MessageId:     uuid.NewString(),
		CorrelationId: correlationID,
	}
	switch value := payload.(type) {
	case *integrationv1.Envelope_AuthResponse:
		envelope.Payload = value
	case *integrationv1.Envelope_Pong:
		envelope.Payload = value
	case *integrationv1.Envelope_SubscribeResponse:
		envelope.Payload = value
	case *integrationv1.Envelope_AckResponse:
		envelope.Payload = value
	case *integrationv1.Envelope_SendBotMessageResponse:
		envelope.Payload = value
	case *integrationv1.Envelope_CreateSessionResponse:
		envelope.Payload = value
	case *integrationv1.Envelope_GetSessionStatusResponse:
		envelope.Payload = value
	case *integrationv1.Envelope_GetBotStatusResponse:
		envelope.Payload = value
	case *integrationv1.Envelope_RequestActionResponse:
		envelope.Payload = value
	case *integrationv1.Envelope_Event:
		envelope.Payload = value
	case *integrationv1.Envelope_Error:
		envelope.Payload = value
	}
	return envelope
}
