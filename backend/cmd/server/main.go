package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/saravana/vaultly/internal/api"
	"github.com/saravana/vaultly/internal/audit"
	"github.com/saravana/vaultly/internal/config"
	"github.com/saravana/vaultly/internal/db"
	"github.com/saravana/vaultly/internal/pii"
)

func main() {
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
	app := api.NewServer(cfg, pool, rdb, keys, box, a)
	go app.RunJanitor(ctx)
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: app.Router(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("vaultly api listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	<-ctx.Done()
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(sctx)
}
