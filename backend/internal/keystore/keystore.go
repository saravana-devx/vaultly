// Package keystore hands out the unwrapped data key of a project. Keys are wrapped by the
// KeyProvider (OpenBao or MASTER_KEY) and cached in memory for a short time.
package keystore

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"vaultly/internal/crypto"
)

type Repository interface {
	WrappedKey(ctx context.Context, projectID uuid.UUID) ([]byte, error)
}

type Service struct {
	keys  crypto.KeyProvider
	repo  Repository
	cache *cache
}

func NewService(keys crypto.KeyProvider, repo Repository, ttl time.Duration) *Service {
	return &Service{keys: keys, repo: repo, cache: newCache(ttl)}
}

func (s *Service) ProjectKey(ctx context.Context, projectID uuid.UUID) ([]byte, error) {
	if key, ok := s.cache.get(projectID); ok {
		return key, nil
	}
	wrapped, err := s.repo.WrappedKey(ctx, projectID)
	if err != nil {
		return nil, err
	}
	key, err := s.keys.UnwrapDEK(ctx, wrapped)
	if err != nil {
		return nil, err
	}
	s.cache.put(projectID, key)
	return key, nil
}

// NewProjectKey returns a fresh data key and its wrapped form for storage.
func (s *Service) NewProjectKey(ctx context.Context) (plain, wrapped []byte, err error) {
	return s.keys.NewDEK(ctx)
}

// Verify proves a stored wrapped key can still be unwrapped (used before restoring a backup).
func (s *Service) Verify(ctx context.Context, wrapped []byte) error {
	_, err := s.keys.UnwrapDEK(ctx, wrapped)
	return err
}

func (s *Service) Forget(projectID uuid.UUID) { s.cache.forget(projectID) }

type Repo struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repo { return &Repo{db: db} }

func (r *Repo) WrappedKey(ctx context.Context, projectID uuid.UUID) ([]byte, error) {
	var wrapped []byte
	err := r.db.QueryRow(ctx, `SELECT encrypted_dek FROM projects WHERE id=$1`, projectID).Scan(&wrapped)
	return wrapped, err
}

type cache struct {
	mu  sync.Mutex
	ttl time.Duration
	m   map[uuid.UUID]entry
}

type entry struct {
	key     []byte
	expires time.Time
}

func newCache(ttl time.Duration) *cache { return &cache{ttl: ttl, m: map[uuid.UUID]entry{}} }

func (c *cache) get(id uuid.UUID) ([]byte, bool) {
	if c.ttl <= 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[id]
	if !ok || time.Now().After(e.expires) {
		delete(c.m, id)
		return nil, false
	}
	return e.key, true
}

func (c *cache) put(id uuid.UUID, key []byte) {
	if c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) > 1024 {
		now := time.Now()
		for k, v := range c.m {
			if now.After(v.expires) {
				delete(c.m, k)
			}
		}
	}
	c.m[id] = entry{key: key, expires: time.Now().Add(c.ttl)}
}

func (c *cache) forget(id uuid.UUID) {
	c.mu.Lock()
	delete(c.m, id)
	c.mu.Unlock()
}
