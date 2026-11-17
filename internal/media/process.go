package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

const Formats = "mov,matroska,webm,wav,mp3,flac,ogg,avi,mpegts,aac"

type LimitedBuffer struct {
	bytes.Buffer
	Limit int
}

func (b *LimitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.Limit - b.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.Buffer.Write(p)
	}
	return n, nil
}

// ProcessError describes a bounded media process failure with enough
// structure to tell an unstartable binary or a blown deadline (runtime
// environment) apart from a non-zero exit on real input.
type ProcessError struct {
	Err    error  // start failure, non-zero exit, or ctx-induced kill
	Stderr string // bounded stderr tail
	Ctx    error  // context error if the deadline or cancel ended the run
}

func (e *ProcessError) Error() string {
	return fmt.Sprintf("media process: %v: %s", e.Err, e.Stderr)
}
func (e *ProcessError) Unwrap() error { return e.Err }

// Exited reports the process genuinely ran and exited non-zero; for probing
// that means the bytes themselves were rejected. A kill from an expired or
// cancelled context is an environment condition, not an exit.
func (e *ProcessError) Exited() bool {
	var exit *exec.ExitError
	return e.Ctx == nil && errors.As(e.Err, &exit)
}
func Run(ctx context.Context, binary string, args []string, limit int) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	var out, errout LimitedBuffer
	out.Limit = limit
	errout.Limit = 8192
	cmd.Stdout = &out
	cmd.Stderr = &errout
	if err := cmd.Run(); err != nil {
		return nil, &ProcessError{Err: err, Stderr: errout.String(), Ctx: ctx.Err()}
	}
	if out.Len() >= limit {
		return nil, fmt.Errorf("media metadata exceeds limit")
	}
	return out.Bytes(), nil
}
