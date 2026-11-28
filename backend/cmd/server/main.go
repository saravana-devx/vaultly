package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"vaultly/internal/audit"
	"vaultly/internal/bootstrap"
	"vaultly/internal/config"
	"vaultly/internal/db"
	"vaultly/internal/demo"
	"vaultly/internal/pii"
)

func main() {
	// seed-demo only talks to the HTTP API, so it needs no config, database or keys.
	if len(os.Args) > 1 && os.Args[1] == "seed-demo" {
		seedDemo()
		return
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatal(err)
	}

	keys, err := buildKeys(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) > 1 && os.Args[1] == "rewrap" {
		if err := rewrapAll(ctx, pool, keys); err != nil {
			log.Fatal(err)
		}
		return
	}
	box, err := pii.Load(ctx, pool, keys)
	if err != nil {
		log.Fatalf("pii keys: %v", err)
	}
	if err := box.Backfill(ctx, pool); err != nil {
		log.Fatalf("pii backfill: %v", err)
	}

	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatal(err)
	}
	rdb := redis.NewClient(opt)
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis: %v", err)
	}

	a := audit.New(pool, cfg.KafkaBrokers, cfg.AuditTopic).WithSealer(box)
	defer a.Close()
	go a.Run(ctx)

	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	app := bootstrap.New(bootstrap.Deps{Config: cfg, DB: pool, Redis: rdb, Keys: keys, PII: box, Audit: a})
	if err := app.Run(ctx, log.Printf); err != nil {
		log.Fatal(err)
	}
}

func seedDemo() {
	base := os.Getenv("DEMO_API_URL")
	if base == "" {
		base = "http://127.0.0.1:8080"
	}
	res, err := demo.Seed(base, func(f string, a ...any) { log.Printf("demo: "+f, a...) })
	if err != nil {
		log.Fatalf("demo: %v", err)
	}
	if res.AlreadySeeded {
		log.Println("demo: already seeded, nothing changed")
		return
	}
	// The token is printed once, like in the UI: it is only for trying the CLI locally.
	fmt.Printf("DEMO_SERVICE_TOKEN=%s\n", res.ServiceToken)
}
