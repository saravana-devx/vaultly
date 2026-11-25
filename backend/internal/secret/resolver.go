package secret

import (
	"context"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"vaultly/internal/crypto"
)

// A reference looks like ${KEY}, ${/path/KEY} or ${env:/path/KEY}.
var refRe = regexp.MustCompile(`\$\{([^${}]+)\}`)

type EnvLookup interface {
	EnvID(ctx context.Context, projectID uuid.UUID, slug string) (uuid.UUID, error)
}

type LiveReader interface {
	Lookup(ctx context.Context, envID uuid.UUID, path, key string) ([]byte, error)
}

// resolver expands references in a value. allow decides which (environment, path) it may read,
// so a service token can only resolve inside its own scope.
type resolver struct {
	ctx       context.Context
	projectID uuid.UUID
	dek       []byte
	allow     func(env, path string) bool
	envs      EnvLookup
	live      LiveReader
	envIDs    map[string]uuid.UUID
}

func newResolver(ctx context.Context, projectID uuid.UUID, dek []byte, allow func(env, path string) bool, envs EnvLookup, live LiveReader) *resolver {
	return &resolver{ctx: ctx, projectID: projectID, dek: dek, allow: allow, envs: envs, live: live, envIDs: map[string]uuid.UUID{}}
}

func (r *resolver) envID(slug string) (uuid.UUID, bool) {
	if id, ok := r.envIDs[slug]; ok {
		return id, true
	}
	id, err := r.envs.EnvID(r.ctx, r.projectID, slug)
	if err != nil {
		return uuid.Nil, false
	}
	r.envIDs[slug] = id
	return id, true
}

func (r *resolver) expand(val, env, path string, stack map[string]bool, depth int) string {
	if depth > 8 || !strings.Contains(val, "${") {
		return val
	}
	return refRe.ReplaceAllStringFunc(val, func(m string) string {
		inner := m[2 : len(m)-1]
		targetEnv := env
		if i := strings.Index(inner, ":"); i > 0 {
			targetEnv, inner = inner[:i], inner[i+1:]
		}
		targetPath, key, absolute := path, inner, strings.HasPrefix(inner, "/")
		if absolute {
			i := strings.LastIndex(inner, "/")
			targetPath, key = inner[:i], inner[i+1:]
			if targetPath == "" {
				targetPath = "/"
			}
		}
		candidates := []string{targetPath}
		if !absolute && targetPath != "/" {
			candidates = append(candidates, "/")
		}
		for _, p := range candidates {
			if v, ok := r.lookup(targetEnv, p, key, stack, depth); ok {
				return v
			}
		}
		return m
	})
}

func (r *resolver) lookup(env, path, key string, stack map[string]bool, depth int) (string, bool) {
	id := env + "|" + path + "|" + key
	if stack[id] || (r.allow != nil && !r.allow(env, path)) {
		return "", false
	}
	envID, ok := r.envID(env)
	if !ok {
		return "", false
	}
	ct, err := r.live.Lookup(r.ctx, envID, path, key)
	if err != nil {
		return "", false
	}
	plain, err := crypto.Decrypt(r.dek, ct, []byte(r.projectID.String()))
	if err != nil {
		return "", false
	}
	stack[id] = true
	defer delete(stack, id)
	return r.expand(string(plain), env, path, stack, depth+1), true
}
