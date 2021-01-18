package exec

import (
	"context"
	"time"
)

type ExecutorOpts struct {
	ExecPath    string
	ExecLogPath string
	Timeout     time.Duration
}

func OptsFromContext(ctx context.Context) *ExecutorOpts {
	// TODO
	return nil
}
