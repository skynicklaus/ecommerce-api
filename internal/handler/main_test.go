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
		// Anonymous goroutine in UpdateProduct that deletes orphaned S3 asset keys
		// when variants are removed; detaches via context.WithoutCancel, 10s timeout.
		// func3 = third function literal in UpdateProduct (after the defer and errgroup closures).
		goleak.IgnoreAnyFunction("github.com/skynicklaus/ecommerce-api/internal/handler.(*V1Handler).UpdateProduct.func3"),
		// Anonymous goroutine in DeleteProduct that batch-deletes all product S3 assets;
		// detaches via context.WithoutCancel, 30s timeout.
		goleak.IgnoreAnyFunction("github.com/skynicklaus/ecommerce-api/internal/handler.(*V1Handler).DeleteProduct.func1"),
		// Standard HTTP transport persistent connection read/write loops
		goleak.IgnoreAnyFunction("net/http.(*persistConn).readLoop"),
		goleak.IgnoreAnyFunction("net/http.(*persistConn).writeLoop"),
	)
}
