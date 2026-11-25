package ratelimit

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func testLimiter(t *testing.T) *SlidingWindow {
	t.Helper()
	url := os.Getenv("REDIS_URL")
	if url == "" {
		url = "redis://localhost:6379/0"
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(opt)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis unavailable: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	return NewSlidingWindow(rdb)
}

func TestAllowsUpToLimitThenBlocks(t *testing.T) {
	l, ctx, key := testLimiter(t), context.Background(), "test:"+uuid.NewString()
	for i := 0; i < 5; i++ {
		r, err := l.Allow(ctx, key, 5, time.Minute)
		if err != nil || !r.Allowed || r.Remaining != int64(4-i) {
			t.Fatalf("request %d: %+v %v", i, r, err)
		}
	}
	r, err := l.Allow(ctx, key, 5, time.Minute)
	if err != nil || r.Allowed || r.Remaining != 0 || !r.ResetAt.After(time.Now()) {
		t.Fatalf("expected block with future reset, got %+v %v", r, err)
	}
}

func TestRejectedRequestsDoNotExtendTheWindow(t *testing.T) {
	l, ctx, key := testLimiter(t), context.Background(), "test:"+uuid.NewString()
	for i := 0; i < 2; i++ {
		l.Allow(ctx, key, 2, 400*time.Millisecond)
	}
	for i := 0; i < 20; i++ {
		l.Allow(ctx, key, 2, 400*time.Millisecond)
	}
	time.Sleep(450 * time.Millisecond)
	if r, _ := l.Allow(ctx, key, 2, 400*time.Millisecond); !r.Allowed {
		t.Fatal("window should have slid past the original two requests")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l, ctx := testLimiter(t), context.Background()
	a, b := "test:"+uuid.NewString(), "test:"+uuid.NewString()
	l.Allow(ctx, a, 1, time.Minute)
	if r, _ := l.Allow(ctx, a, 1, time.Minute); r.Allowed {
		t.Fatal("a should be exhausted")
	}
	if r, _ := l.Allow(ctx, b, 1, time.Minute); !r.Allowed {
		t.Fatal("b should be unaffected")
	}
}
