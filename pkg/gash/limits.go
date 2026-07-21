package gash

import (
	"fmt"
	"sync/atomic"
	"time"
)

type LimitProfile string

const (
	NormalProfile   LimitProfile = "normal"
	HardenedProfile LimitProfile = "hardened"
)

// Limits is the Go representation of just-bash's top-level shell limits.
// Zero values inherit the selected profile.
type Limits struct {
	MaxSourceBytes     int64
	MaxExecDepth       int
	MaxCallDepth       int
	MaxCommandCount    int64
	MaxInputBytes      int64
	MaxOutputBytes     int
	MaxExecutionTime   time.Duration
	MaxFileSystemBytes int64
}

func normalLimits() Limits {
	return Limits{MaxSourceBytes: 64 << 20, MaxExecDepth: 64, MaxCallDepth: 100, MaxCommandCount: 100_000, MaxInputBytes: 512 << 20, MaxOutputBytes: 256 << 20, MaxExecutionTime: time.Hour, MaxFileSystemBytes: 1 << 30}
}

func hardenedLimits() Limits {
	v := normalLimits()
	v.MaxSourceBytes = 8 << 20
	v.MaxCommandCount = 10_000
	v.MaxInputBytes = 32 << 20
	v.MaxOutputBytes = 10 << 20
	v.MaxExecutionTime = 30 * time.Second
	v.MaxFileSystemBytes = 128 << 20
	return v
}

func resolveLimits(user Limits, profile LimitProfile) (Limits, error) {
	var out Limits
	switch profile {
	case "", NormalProfile:
		out = normalLimits()
	case HardenedProfile:
		out = hardenedLimits()
	default:
		return Limits{}, fmt.Errorf("limit profile must be %q or %q", NormalProfile, HardenedProfile)
	}
	if user.MaxSourceBytes != 0 {
		out.MaxSourceBytes = user.MaxSourceBytes
	}
	if user.MaxExecDepth != 0 {
		out.MaxExecDepth = user.MaxExecDepth
	}
	if user.MaxCallDepth != 0 {
		out.MaxCallDepth = user.MaxCallDepth
	}
	if user.MaxCommandCount != 0 {
		out.MaxCommandCount = user.MaxCommandCount
	}
	if user.MaxInputBytes != 0 {
		out.MaxInputBytes = user.MaxInputBytes
	}
	if user.MaxOutputBytes != 0 {
		out.MaxOutputBytes = user.MaxOutputBytes
	}
	if user.MaxExecutionTime != 0 {
		out.MaxExecutionTime = user.MaxExecutionTime
	}
	if user.MaxFileSystemBytes != 0 {
		out.MaxFileSystemBytes = user.MaxFileSystemBytes
	}
	if out.MaxSourceBytes < 0 || out.MaxExecDepth < 0 || out.MaxCallDepth < 0 || out.MaxCommandCount < 0 || out.MaxInputBytes < 0 || out.MaxOutputBytes < 0 || out.MaxExecutionTime < 0 || out.MaxFileSystemBytes < 0 {
		return Limits{}, fmt.Errorf("execution limits must be non-negative")
	}
	return out, nil
}

type executionScope struct {
	limits   Limits
	commands atomic.Int64
	input    atomic.Int64
}

func (s *executionScope) chargeCommand() error {
	if s.commands.Add(1) > s.limits.MaxCommandCount {
		return fmt.Errorf("too many commands executed (>%d)", s.limits.MaxCommandCount)
	}
	return nil
}

func (s *executionScope) consumeInput(n int64) error {
	if s.input.Add(n) > s.limits.MaxInputBytes {
		return fmt.Errorf("aggregate input size limit exceeded (%d bytes)", s.limits.MaxInputBytes)
	}
	return nil
}

type outputBudget struct {
	used     atomic.Int64
	maximum  int64
	exceeded atomic.Bool
}

func (b *outputBudget) take(n int) int {
	if n <= 0 {
		return 0
	}
	for {
		used := b.used.Load()
		remaining := b.maximum - used
		if remaining <= 0 {
			b.exceeded.Store(true)
			return 0
		}
		take := int64(n)
		if take > remaining {
			take = remaining
		}
		if b.used.CompareAndSwap(used, used+take) {
			if take < int64(n) {
				b.exceeded.Store(true)
			}
			return int(take)
		}
	}
}
