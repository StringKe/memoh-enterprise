package executorsvc

import (
	"time"

	"github.com/memohai/memoh/internal/serviceauth"
)

func authorizeWorkspaceLease(claims serviceauth.Claims, lease serviceauth.RunLease, scope string, now time.Time) error {
	return serviceauth.RequireWorkspaceLease(claims, lease, scope, now)
}
