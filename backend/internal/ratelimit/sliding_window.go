package ratelimit

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

//go:embed lua/sliding_window.lua
var slidingWindowSrc string

var slidingWindowScript = redis.NewScript(slidingWindowSrc)

type SlidingWindow struct {
	rdb *redis.Client
}

func NewSlidingWindow(rdb *redis.Client) *SlidingWindow {
	return &SlidingWindow{rdb: rdb}
}

func (s *SlidingWindow) Allow(ctx context.Context, key string, limit int, window time.Duration) (*Result, error) {
	res, err := slidingWindowScript.Run(ctx, s.rdb, []string{key}, limit, window.Milliseconds(), uuid.NewString()).Slice()
	if err != nil {
		return nil, err
	}
	if len(res) != 3 {
		return nil, fmt.Errorf("unexpected script reply: %v", res)
	}
	return &Result{
		Allowed:   res[0].(int64) == 1,
		Remaining: res[1].(int64),
		ResetAt:   time.UnixMilli(res[2].(int64)),
	}, nil
}

var _ Limiter = (*SlidingWindow)(nil)
