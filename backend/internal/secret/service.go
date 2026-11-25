package secret

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"vaultly/internal/access"
	"vaultly/internal/apperr"
	"vaultly/internal/audit"
	"vaultly/internal/crypto"
	"vaultly/internal/keystore"
	"vaultly/internal/pathutil"
	"vaultly/internal/pii"
	"vaultly/internal/principal"
	"vaultly/internal/tokenutil"
)

type SecretRepository interface {
	Get(ctx context.Context, projectID, id uuid.UUID) (*Stored, error)
	List(ctx context.Context, envID uuid.UUID, path, tag string) ([]Secret, error)
	Search(ctx context.Context, projectID uuid.UUID, envIDs []uuid.UUID, like string) ([]Secret, error)
	Upsert(ctx context.Context, w Write) (Secret, bool, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Versions(ctx context.Context, id uuid.UUID) ([]VersionInfo, error)
	VersionCiphertext(ctx context.Context, id uuid.UUID, version int) ([]byte, error)
	Live(ctx context.Context, envID uuid.UUID, path string, recursive bool) ([]LiveRow, error)
	Lookup(ctx context.Context, envID uuid.UUID, path, key string) ([]byte, error)
	MarkExpired(ctx context.Context) ([]Expired, error)
}

type Service struct {
	repo   SecretRepository
	access *access.Service
	keys   *keystore.Service
	pii    *pii.Box
	audit  *audit.Service
}

func NewService(repo SecretRepository, access *access.Service, keys *keystore.Service, box *pii.Box, audit *audit.Service) *Service {
	return &Service{repo: repo, access: access, keys: keys, pii: box, audit: audit}
}

// scope is a project the caller was already authorised for.
type scope struct {
	p     *access.Project
	actor principal.Actor
}

func (s *Service) open(ctx context.Context, a principal.Actor, projectID uuid.UUID) (*scope, error) {
	p, err := s.access.Project(ctx, a.UserID, projectID, access.RoleViewer)
	if err != nil {
		return nil, err
	}
	return &scope{p: p, actor: a}, nil
}

func (s *Service) need(ctx context.Context, sc *scope, env, role string) error {
	return s.access.NeedEnv(ctx, sc.p, sc.actor.UserID, env, role)
}

func (s *Service) tidy(sec *Secret) { sec.UpdatedBy = s.pii.OpenLabel(sec.UpdatedBy) }

func (s *Service) userAllow(ctx context.Context, sc *scope) func(env, path string) bool {
	cache := map[string]bool{}
	return func(env, _ string) bool {
		if v, ok := cache[env]; ok {
			return v
		}
		role, err := s.access.EnvRole(ctx, sc.p, sc.actor.UserID, env)
		v := err == nil && access.Rank(role) >= access.Rank(access.RoleViewer)
		cache[env] = v
		return v
	}
}

func (s *Service) encrypt(ctx context.Context, projectID uuid.UUID, value string) ([]byte, error) {
	dek, err := s.keys.ProjectKey(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return crypto.Encrypt(dek, []byte(value), []byte(projectID.String()))
}

func (s *Service) decrypt(ctx context.Context, projectID uuid.UUID, ct []byte) (string, error) {
	dek, err := s.keys.ProjectKey(ctx, projectID)
	if err != nil {
		return "", err
	}
	pt, err := crypto.Decrypt(dek, ct, []byte(projectID.String()))
	return string(pt), err
}

func (s *Service) write(ctx context.Context, w Write, value string) (Secret, bool, error) {
	ct, err := s.encrypt(ctx, w.ProjectID, value)
	if err != nil {
		return Secret{}, false, err
	}
	w.Ciphertext = ct
	sec, created, err := s.repo.Upsert(ctx, w)
	sec.UpdatedBy = w.Actor
	s.tidy(&sec)
	return sec, created, err
}

func (s *Service) List(ctx context.Context, a principal.Actor, projectID uuid.UUID, q Query) (*Listing, error) {
	sc, err := s.open(ctx, a, projectID)
	if err != nil {
		return nil, err
	}
	if err := s.need(ctx, sc, q.Env, access.RoleViewer); err != nil {
		return nil, err
	}
	path, ok := pathutil.Norm(q.Path)
	if !ok {
		return nil, ErrInvalidPath
	}
	envID, err := s.access.EnvID(ctx, projectID, q.Env)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.List(ctx, envID, path, strings.ToLower(q.Tag))
	if err != nil {
		return nil, err
	}
	out := &Listing{Secrets: []Secret{}, Path: path}
	folders := map[string]bool{}
	for _, r := range rows {
		if r.Path == path {
			s.tidy(&r)
			out.Secrets = append(out.Secrets, r)
			continue
		}
		rest := strings.TrimPrefix(strings.TrimPrefix(r.Path, strings.TrimRight(path, "/")), "/")
		folders[strings.SplitN(rest, "/", 2)[0]] = true
	}
	out.Folders = make([]string, 0, len(folders))
	for f := range folders {
		out.Folders = append(out.Folders, f)
	}
	sort.Strings(out.Folders)
	return out, nil
}

func (s *Service) Search(ctx context.Context, a principal.Actor, projectID uuid.UUID, env, query string) (*SearchResults, error) {
	sc, err := s.open(ctx, a, projectID)
	if err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if len(query) < 2 {
		return &SearchResults{Results: []Secret{}}, nil
	}
	readable, err := s.access.ReadableEnvs(ctx, sc.p, a.UserID)
	if err != nil {
		return nil, err
	}
	ids := []uuid.UUID{}
	for slug, id := range readable {
		if env == "" || env == slug {
			ids = append(ids, id)
		}
	}
	like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query) + "%"
	rows, err := s.repo.Search(ctx, projectID, ids, like)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		s.tidy(&rows[i])
	}
	return &SearchResults{Results: rows}, nil
}

func (s *Service) load(ctx context.Context, a principal.Actor, projectID, id uuid.UUID, need string) (*scope, *Stored, error) {
	sc, err := s.open(ctx, a, projectID)
	if err != nil {
		return nil, nil, err
	}
	st, err := s.repo.Get(ctx, projectID, id)
	if err != nil {
		return nil, nil, err
	}
	if err := s.need(ctx, sc, st.Env, need); err != nil {
		return nil, nil, err
	}
	return sc, st, nil
}

func (s *Service) Get(ctx context.Context, a principal.Actor, projectID, id uuid.UUID) (*Secret, error) {
	sc, st, err := s.load(ctx, a, projectID, id, access.RoleViewer)
	if err != nil {
		return nil, err
	}
	value, err := s.decrypt(ctx, projectID, st.Ciphertext)
	if err != nil {
		return nil, err
	}
	sec := st.Secret
	s.tidy(&sec)
	sec.Value = &value
	if strings.Contains(value, "${") {
		res, err := s.resolver(ctx, projectID, s.userAllow(ctx, sc))
		if err != nil {
			return nil, err
		}
		expanded := res.expand(value, sec.Env, sec.Path, map[string]bool{sec.Env + "|" + sec.Path + "|" + sec.Key: true}, 0)
		if expanded != value {
			sec.Resolved = &expanded
		}
	}
	s.audit.Record(ctx, a, sc.p.OrgID, &projectID, "secret.read", pathutil.Join(sec.Path, sec.Key), map[string]any{"env": sec.Env})
	return &sec, nil
}

func (s *Service) Put(ctx context.Context, a principal.Actor, projectID uuid.UUID, req *PutRequest) (*Secret, error) {
	sc, err := s.open(ctx, a, projectID)
	if err != nil {
		return nil, err
	}
	path, ok := pathutil.Norm(req.Path)
	if !ok || !keyRe.MatchString(req.Key) {
		return nil, ErrInvalidKeyPath
	}
	if len(req.Value) > maxValueLen {
		return nil, ErrValueTooLarge
	}
	if req.ExpiresAt != nil && req.ExpiresAt.Before(time.Now()) {
		return nil, ErrExpiryPast
	}
	if req.Tags != nil {
		t, ok := normTags(*req.Tags)
		if !ok {
			return nil, ErrInvalidTags
		}
		req.Tags = &t
	}
	if err := s.need(ctx, sc, req.Env, access.RoleDeveloper); err != nil {
		return nil, err
	}
	envID, err := s.access.EnvID(ctx, projectID, req.Env)
	if err != nil {
		return nil, err
	}
	sec, created, err := s.write(ctx, Write{ProjectID: projectID, EnvID: envID, Path: path, Key: req.Key, Comment: req.Comment,
		Actor: s.pii.Label(a.Label()), Tags: req.Tags, Expires: req.ExpiresAt, ClearExpiry: req.ClearExpiry}, req.Value)
	if err != nil {
		return nil, err
	}
	sec.Env = req.Env
	action := "secret.update"
	if created {
		action = "secret.create"
	}
	s.audit.Record(ctx, a, sc.p.OrgID, &projectID, action, pathutil.Join(path, req.Key), map[string]any{"env": req.Env, "version": sec.Version})
	return &sec, nil
}

func (s *Service) Delete(ctx context.Context, a principal.Actor, projectID, id uuid.UUID) error {
	sc, st, err := s.load(ctx, a, projectID, id, access.RoleDeveloper)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, st.ID); err != nil {
		return err
	}
	s.audit.Record(ctx, a, sc.p.OrgID, &projectID, "secret.delete", pathutil.Join(st.Path, st.Key), map[string]any{"env": st.Env})
	return nil
}

func (s *Service) Versions(ctx context.Context, a principal.Actor, projectID, id uuid.UUID) ([]VersionInfo, error) {
	_, st, err := s.load(ctx, a, projectID, id, access.RoleViewer)
	if err != nil {
		return nil, err
	}
	out, err := s.repo.Versions(ctx, st.ID)
	for i := range out {
		out[i].Actor = s.pii.OpenLabel(out[i].Actor)
	}
	return out, err
}

func (s *Service) Rollback(ctx context.Context, a principal.Actor, projectID, id uuid.UUID, req *RollbackRequest) (*Secret, error) {
	sc, st, err := s.load(ctx, a, projectID, id, access.RoleDeveloper)
	if err != nil {
		return nil, err
	}
	ct, err := s.repo.VersionCiphertext(ctx, st.ID, req.Version)
	if err != nil {
		return nil, err
	}
	value, err := s.decrypt(ctx, projectID, ct)
	if err != nil {
		return nil, err
	}
	sec, err := s.rewrite(ctx, a, sc, st, value, "rollback")
	if err != nil {
		return nil, err
	}
	s.audit.Record(ctx, a, sc.p.OrgID, &projectID, "secret.rollback", pathutil.Join(st.Path, st.Key),
		map[string]any{"env": st.Env, "to_version": req.Version, "version": sec.Version})
	return sec, nil
}

func (s *Service) Rotate(ctx context.Context, a principal.Actor, projectID, id uuid.UUID, req *RotateRequest) (*Secret, error) {
	sc, st, err := s.load(ctx, a, projectID, id, access.RoleDeveloper)
	if err != nil {
		return nil, err
	}
	length := req.Length
	if length < 16 || length > 256 {
		length = 32
	}
	value, err := tokenutil.RandomString(length)
	if err != nil {
		return nil, err
	}
	sec, err := s.rewrite(ctx, a, sc, st, value, "rotate")
	if err != nil {
		return nil, err
	}
	s.audit.Record(ctx, a, sc.p.OrgID, &projectID, "secret.rotate", pathutil.Join(st.Path, st.Key), map[string]any{"env": st.Env, "version": sec.Version})
	sec.Value = &value
	return sec, nil
}

func (s *Service) rewrite(ctx context.Context, a principal.Actor, sc *scope, st *Stored, value, action string) (*Secret, error) {
	envID, err := s.access.EnvID(ctx, sc.p.ID, st.Env)
	if err != nil {
		return nil, err
	}
	sec, _, err := s.write(ctx, Write{ProjectID: sc.p.ID, EnvID: envID, Path: st.Path, Key: st.Key, Comment: st.Comment,
		Actor: s.pii.Label(a.Label()), Action: action}, value)
	if err != nil {
		return nil, err
	}
	sec.Env = st.Env
	return &sec, nil
}

func (s *Service) Import(ctx context.Context, a principal.Actor, projectID uuid.UUID, req *ImportRequest) (*ImportResult, error) {
	sc, err := s.open(ctx, a, projectID)
	if err != nil {
		return nil, err
	}
	path, ok := pathutil.Norm(req.Path)
	envID, err := s.access.EnvID(ctx, projectID, req.Env)
	if !ok || err != nil {
		if err != nil && !errors.Is(err, access.ErrNotFound) {
			return nil, err
		}
		return nil, ErrInvalidEnvPath
	}
	if err := s.need(ctx, sc, req.Env, access.RoleDeveloper); err != nil {
		return nil, err
	}
	pairs := parseDotenv(req.Content)
	if len(pairs) == 0 {
		return nil, ErrNoPairs
	}
	res := &ImportResult{}
	label := s.pii.Label(a.Label())
	for _, kv := range pairs {
		if !keyRe.MatchString(kv[0]) || len(kv[1]) > maxValueLen {
			res.Skipped++
			continue
		}
		_, created, err := s.write(ctx, Write{ProjectID: projectID, EnvID: envID, Path: path, Key: kv[0], Actor: label}, kv[1])
		if err != nil {
			return nil, err
		}
		if created {
			res.Created++
		} else {
			res.Updated++
		}
	}
	s.audit.Record(ctx, a, sc.p.OrgID, &projectID, "secret.import", path, map[string]any{"env": req.Env, "created": res.Created, "updated": res.Updated})
	return res, nil
}

func (s *Service) Export(ctx context.Context, a principal.Actor, projectID uuid.UUID, q ExportQuery) ([]Pair, error) {
	sc, err := s.open(ctx, a, projectID)
	if err != nil {
		return nil, err
	}
	path, ok := pathutil.Norm(q.Path)
	envID, err := s.access.EnvID(ctx, projectID, q.Env)
	if !ok || err != nil {
		if err != nil && !errors.Is(err, access.ErrNotFound) {
			return nil, err
		}
		return nil, ErrInvalidEnvPath
	}
	if err := s.need(ctx, sc, q.Env, access.RoleViewer); err != nil {
		return nil, err
	}
	pairs, err := s.collect(ctx, projectID, envID, q.Env, path, q.Recursive, q.Resolve, s.userAllow(ctx, sc))
	if err != nil {
		return nil, err
	}
	s.audit.Record(ctx, a, sc.p.OrgID, &projectID, "secret.export", path, map[string]any{"env": q.Env, "count": len(pairs)})
	return pairs, nil
}

// Fetch is the machine read: a service token sees only its environment and path prefix.
func (s *Service) Fetch(ctx context.Context, a principal.Actor, requestedPath string, recursive, resolve bool) ([]Pair, error) {
	t := a.Token
	path, err := tokenPath(t, requestedPath)
	if err != nil {
		return nil, err
	}
	allow := func(env, p string) bool { return env == t.EnvSlug && pathutil.Within(t.PathPrefix, p) }
	pairs, err := s.collect(ctx, t.ProjectID, t.EnvID, t.EnvSlug, path, recursive, resolve, allow)
	if err != nil {
		return nil, err
	}
	s.audit.Record(ctx, a, t.OrgID, &t.ProjectID, "secret.fetch", path, map[string]any{"count": len(pairs), "env": t.EnvSlug})
	return pairs, nil
}

func (s *Service) MachinePut(ctx context.Context, a principal.Actor, req *MachinePutRequest) (*MachineWriteResult, error) {
	t := a.Token
	if !t.CanWrite {
		return nil, ErrReadOnlyToken
	}
	if !keyRe.MatchString(req.Key) || len(req.Value) > maxValueLen {
		return nil, ErrInvalidKeyPath
	}
	path, err := tokenPath(t, req.Path)
	if err != nil {
		return nil, err
	}
	sec, created, err := s.write(ctx, Write{ProjectID: t.ProjectID, EnvID: t.EnvID, Path: path, Key: req.Key, Comment: req.Comment,
		Actor: s.pii.Label(a.Label())}, req.Value)
	if err != nil {
		return nil, err
	}
	action := "secret.update"
	if created {
		action = "secret.create"
	}
	s.audit.Record(ctx, a, t.OrgID, &t.ProjectID, action, pathutil.Join(path, req.Key), map[string]any{"version": sec.Version, "env": t.EnvSlug})
	return &MachineWriteResult{Key: sec.Key, Path: sec.Path, Version: sec.Version}, nil
}

func tokenPath(t *principal.ServiceToken, requested string) (string, error) {
	if requested == "" {
		requested = t.PathPrefix
	}
	path, ok := pathutil.Norm(requested)
	if !ok || !pathutil.Within(t.PathPrefix, path) {
		return "", apperr.Forbidden(fmt.Sprintf(MsgScopeFmt, t.PathPrefix))
	}
	return path, nil
}

func (s *Service) resolver(ctx context.Context, projectID uuid.UUID, allow func(env, path string) bool) (*resolver, error) {
	dek, err := s.keys.ProjectKey(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return newResolver(ctx, projectID, dek, allow, s.access, s.repo), nil
}

// collect decrypts live secrets at path (and below if recursive); deeper paths win on key collisions.
func (s *Service) collect(ctx context.Context, projectID, envID uuid.UUID, envSlug, path string, recursive, resolve bool, allow func(env, path string) bool) ([]Pair, error) {
	rows, err := s.repo.Live(ctx, envID, path, recursive)
	if err != nil {
		return nil, err
	}
	dek, err := s.keys.ProjectKey(ctx, projectID)
	if err != nil {
		return nil, err
	}
	var res *resolver
	if resolve {
		res = newResolver(ctx, projectID, dek, allow, s.access, s.repo)
	}
	idx := map[string]int{}
	var out []Pair
	for _, r := range rows {
		pt, err := crypto.Decrypt(dek, r.Ciphertext, []byte(projectID.String()))
		if err != nil {
			return nil, err
		}
		v := string(pt)
		if res != nil {
			v = res.expand(v, envSlug, r.Path, map[string]bool{envSlug + "|" + r.Path + "|" + r.Key: true}, 0)
		}
		if i, seen := idx[r.Key]; seen {
			out[i][1] = v
		} else {
			idx[r.Key] = len(out)
			out = append(out, Pair{r.Key, v})
		}
	}
	return out, nil
}

// Snapshot returns every live secret at path and below with references resolved, for system
// jobs (integration sync) that act without a user or token.
func (s *Service) Snapshot(ctx context.Context, projectID, envID uuid.UUID, envSlug, path string) (map[string]string, error) {
	pairs, err := s.collect(ctx, projectID, envID, envSlug, path, true, true, nil)
	if err != nil {
		return nil, err
	}
	return ToMap(pairs), nil
}

// AnnounceExpired publishes one audit event for each secret that has just expired.
func (s *Service) AnnounceExpired(ctx context.Context) {
	expired, err := s.repo.MarkExpired(ctx)
	if err != nil {
		return
	}
	for _, e := range expired {
		pid := e.ProjectID
		s.audit.Publish(ctx, audit.Event{OrgID: e.OrgID, ProjectID: &pid, ActorType: "system", ActorID: "janitor", ActorLabel: "system",
			Action: "secret.expired", Resource: pathutil.Join(e.Path, e.Key), Metadata: map[string]any{"env": e.Env}})
	}
}
