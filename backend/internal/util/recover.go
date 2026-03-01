package util

import (
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
	"time"
)

// SafeGo runs fn in a new goroutine with panic recovery.
// If fn panics, the panic is logged with the goroutine name and stack trace.
func SafeGo(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("goroutine panicked",
					"name", name,
					"panic", r,
					"stack", string(debug.Stack()),
				)
			}
		}()
		fn()
	}()
}

// PanicCount tracks total recovered panics across all workers.
var PanicCount atomic.Int64

// RecoverWorker is intended to be deferred at the top of each worker loop
// iteration. It recovers from panics, logs the stack trace, increments the
// global panic counter, and sleeps briefly to avoid tight panic loops.
func RecoverWorker(name string) {
	if r := recover(); r != nil {
		PanicCount.Add(1)
		slog.Error("worker panic recovered",
			"worker", name,
			"panic", r,
			"stack", string(debug.Stack()),
			"total_panics", PanicCount.Load(),
		)
		time.Sleep(2 * time.Second)
	}
}

// RecoverWorkerJob is like RecoverWorker but also returns the panic value as
// an error string so callers can mark the specific job as failed.
func RecoverWorkerJob(name string, panicErr *error) {
	if r := recover(); r != nil {
		PanicCount.Add(1)
		slog.Error("worker job panic recovered",
			"worker", name,
			"panic", r,
			"stack", string(debug.Stack()),
			"total_panics", PanicCount.Load(),
		)
		if panicErr != nil {
			*panicErr = fmt.Errorf("panic: %v", r)
		}
		time.Sleep(2 * time.Second)
	}
}
