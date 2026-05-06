package connectapi

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
)

func TestProviderServiceGetOAuthStatusRequiresProviderID(t *testing.T) {
	t.Parallel()

	service := newTestProviderService(&fakeProviderDBTX{})

	_, err := service.GetProviderOauthStatus(context.Background(), connect.NewRequest(&privatev1.GetProviderOauthStatusRequest{}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want %v, err = %v", connect.CodeOf(err), connect.CodeInvalidArgument, err)
	}
}

func TestProviderServiceStartOAuthMapsProviderNotFound(t *testing.T) {
	t.Parallel()

	service := newTestProviderService(&fakeProviderDBTX{})

	_, err := service.StartProviderOauth(context.Background(), connect.NewRequest(&privatev1.StartProviderOauthRequest{
		ProviderId: "550e8400-e29b-41d4-a716-446655440000",
	}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("code = %v, want %v, err = %v", connect.CodeOf(err), connect.CodeNotFound, err)
	}
}
