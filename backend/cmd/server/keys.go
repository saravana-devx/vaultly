package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"vaultly/internal/config"
	"vaultly/internal/crypto"
	"vaultly/internal/pii"
)

// buildKeys selects the root of trust. With KEY_PROVIDER=openbao the wrapping key lives
// inside OpenBao; MASTER_KEY, if still set, is only used to read keys not yet migrated.
func buildKeys(ctx context.Context, cfg *config.Config) (crypto.KeyProvider, error) {
	var local *crypto.Local
	if cfg.MasterKey != "" {
		l, err := crypto.NewLocal(cfg.MasterKey)
		if err != nil {
			return nil, err
		}
		local = l
	}
	if cfg.KeyProvider == "local" {
		return local, nil
	}

	token := func() (string, error) {
		if cfg.BaoToken != "" {
			return cfg.BaoToken, nil
		}
		b, err := os.ReadFile(cfg.BaoTokenFile)
		if err != nil {
			return "", fmt.Errorf("read BAO_TOKEN_FILE: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	tr := crypto.NewTransit(cfg.BaoAddr, cfg.BaoMount, cfg.BaoKey, token, nil)

	// OpenBao may still be starting or unsealing; wait for it rather than crash-looping.
	var err error
	for i := 0; i < 45; i++ {
		if err = tr.Check(ctx); err == nil {
			break
		}
		if i == 0 {
			log.Printf("keys: waiting for OpenBao (%v)", err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if err != nil {
		return nil, fmt.Errorf("openbao is not usable: %w", err)
	}
	if cfg.BaoTokenFile != "" {
		go tr.RunRenewer(ctx, time.Hour)
	}
	log.Printf("keys: using OpenBao Transit at %s (key %q, legacy MASTER_KEY %s)", cfg.BaoAddr, cfg.BaoKey, map[bool]string{true: "present", false: "absent"}[local != nil])
	return &crypto.Hybrid{Primary: tr, Legacy: local}, nil
}

// rewrapAll moves every wrapped key (projects and PII keys) under the provider's current key.
// Plaintext data keys never leave OpenBao when the key is already wrapped by it.
func rewrapAll(ctx context.Context, pool *pgxpool.Pool, kp crypto.KeyProvider) error {
	rows, err := pool.Query(ctx, `SELECT id, encrypted_dek FROM projects`)
	if err != nil {
		return err
	}
	type p struct {
		id string
		w  []byte
	}
	var all []p
	for rows.Next() {
		var x p
		if rows.Scan(&x.id, &x.w) == nil {
			all = append(all, x)
		}
	}
	rows.Close()
	moved := 0
	for _, x := range all {
		nw, err := kp.Rewrap(ctx, x.w)
		if err != nil {
			return fmt.Errorf("project %s: %w", x.id, err)
		}
		tag, err := pool.Exec(ctx, `UPDATE projects SET encrypted_dek=$2 WHERE id=$1 AND encrypted_dek=$3`, x.id, nw, x.w)
		if err != nil {
			return err
		}
		moved += int(tag.RowsAffected())
	}
	n, err := pii.RewrapKeys(ctx, pool, kp)
	if err != nil {
		return err
	}
	log.Printf("rewrap: %d project keys and %d application keys now wrapped by %s", moved, n, kp.Name())
	return nil
}
