package handler

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// cleanupCommittedUpload is a named fire-and-forget goroutine launched after
		// S3 copy; it detaches via context.WithoutCancel and has a 15s timeout.
		goleak.IgnoreAnyFunction("github.com/skynicklaus/ecommerce-api/internal/handler.(*V1Handler).cleanupCommittedUpload"),
		// cleanupUnusedAssetKeys is a named fire-and-forget goroutine launched after
		// product update; it detaches via context.WithoutCancel and has a 10s timeout.
		goleak.IgnoreAnyFunction("github.com/skynicklaus/ecommerce-api/internal/handler.(*V1Handler).cleanupUnusedAssetKeys"),
		// Standard HTTP transport persistent connection read/write loops
		goleak.IgnoreAnyFunction("net/http.(*persistConn).readLoop"),
		goleak.IgnoreAnyFunction("net/http.(*persistConn).writeLoop"),
	)
}
