package connector

import (
	"errors"
	"time"

	"github.com/memohai/memoh/internal/channel"
)

const (
	DefaultLeaseTTL      = 30 * time.Second
	DefaultRenewInterval = 10 * time.Second
)

var (
	ErrLeaseHeld         = errors.New("connector lease already held")
	ErrLeaseNotFound     = errors.New("connector lease not found")
	ErrLeaseExpired      = errors.New("connector lease expired")
	ErrLeaseStale        = errors.New("connector lease stale")
	ErrInvalidLeaseToken = errors.New("invalid connector lease token")
)

type Instance struct {
	OwnerID         string
	OwnerInstanceID string
	ChannelType     channel.ChannelType
	RegisteredAt    time.Time
	Metadata        map[string]string
}

type LeaseToken struct {
	ChannelConfigID string
	ChannelType     channel.ChannelType
	OwnerID         string
	OwnerInstanceID string
	LeaseVersion    int64
	AcquiredAt      time.Time
	ExpiresAt       time.Time
}

type StatusState string

const (
	StatusStateUnknown   StatusState = "unknown"
	StatusStateHealthy   StatusState = "healthy"
	StatusStateUnhealthy StatusState = "unhealthy"
	StatusStateStopped   StatusState = "stopped"
)

type Status struct {
	Token     LeaseToken
	State     StatusState
	LastError string
	UpdatedAt time.Time
}

type RegisterRequest struct {
	OwnerID         string
	OwnerInstanceID string
	ChannelType     channel.ChannelType
	Metadata        map[string]string
}

type RegisterResponse struct {
	Instance Instance
}

type AcquireLeaseRequest struct {
	ChannelConfigID string
	ChannelType     channel.ChannelType
	OwnerID         string
	OwnerInstanceID string
}

type AcquireLeaseResponse struct {
	Lease LeaseToken
}

type RenewLeaseRequest struct {
	Token LeaseToken
}

type RenewLeaseResponse struct {
	Lease LeaseToken
}

type ReleaseLeaseRequest struct {
	Token LeaseToken
}

type ReleaseLeaseResponse struct{}

type ReportStatusRequest struct {
	Status Status
}

type ReportStatusResponse struct{}

type InboundEvent struct {
	Token      LeaseToken
	Config     channel.ChannelConfig
	Message    channel.InboundMessage
	ReceivedAt time.Time
}

type StreamInboundResponse struct{}

type OutboundCommand struct {
	ID      string
	Token   LeaseToken
	Target  string
	Message channel.Message
}

type StreamOutboundRequest struct {
	Token LeaseToken
}
