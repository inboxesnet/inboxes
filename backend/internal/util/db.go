package util

import (
	"context"
	"time"
)

// DBCtx returns a child context with a 30-second timeout for database operations.
func DBCtx(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 30*time.Second)
}
