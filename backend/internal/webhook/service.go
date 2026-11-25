package webhook

import (
	"context"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"vaultly/internal/access"
	"vaultly/internal/audit"
	"vaultly/internal/crypto"
	"vaultly/internal/keystore"
	"vaultly/internal/principal"
	"vaultly/internal/tokenutil"
)

var eventPatternRe = regexp.MustCompile(`^(\*|[a-z_]+\.(\*|[a-z_]+))$`)

type WebhookRepository interface {
	List(ctx context.Context, projectID uuid.UUID) ([]View, error)
	Create(ctx context.Context, projectID uuid.UUID, name, url string, secretEnc []byte, events []string) (uuid.UUID, error)
	Get(ctx context.Context, projectID, id uuid.UUID) (*HookRow, error)
	Update(ctx context.Context, id uuid.UUID, u Update) error
	Delete(ctx context.Context, id uuid.UUID) error
	Deliveries(ctx context.Context, id uuid.UUID) ([]Delivery, error)
	RecordDelivery(ctx context.Context, hookID uuid.UUID, ev audit.Event, r Result)
}

type Service struct {
	repo         WebhookRepository
	access       *access.Service
	keys         *keystore.Service
	dispatcher   *Dispatcher
	audit        *audit.Service
	allowPrivate bool
}

func NewService(repo WebhookRepository, access *access.Service, keys *keystore.Service, d *Dispatcher, audit *audit.Service, allowPrivate bool) *Service {
	return &Service{repo: repo, access: access, keys: keys, dispatcher: d, audit: audit, allowPrivate: allowPrivate}
}

func (s *Service) validURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" {
		return ErrInvalidURL
	}
	if !s.allowPrivate {
		if ip := net.ParseIP(u.Hostname()); (ip != nil && BlockedIP(ip)) || strings.EqualFold(u.Hostname(), "localhost") {
			return ErrPrivateURL
		}
	}
	return nil
}

func normEvents(in []string) ([]string, bool) {
	if len(in) == 0 {
		return []string{"*"}, true
	}
	out := []string{}
	for _, e := range in {
		e = strings.TrimSpace(e)
		if !eventPatternRe.MatchString(e) {
			return nil, false
		}
		out = append(out, e)
	}
	return out, len(out) <= 30
}

func (s *Service) List(ctx context.Context, a principal.Actor, projectID uuid.UUID) ([]View, error) {
	if _, err := s.access.Project(ctx, a.UserID, projectID, access.RoleAdmin); err != nil {
		return nil, err
	}
	return s.repo.List(ctx, projectID)
}

func (s *Service) Create(ctx context.Context, a principal.Actor, projectID uuid.UUID, req *CreateRequest) (*Created, error) {
	p, err := s.access.Project(ctx, a.UserID, projectID, access.RoleAdmin)
	if err != nil {
		return nil, err
	}
	if err := s.validURL(req.URL); err != nil {
		return nil, err
	}
	events, ok := normEvents(req.Events)
	if !ok {
		return nil, ErrInvalidEvents
	}
	dek, err := s.keys.ProjectKey(ctx, projectID)
	if err != nil {
		return nil, err
	}
	secret, _ := tokenutil.New("whsec_")
	enc, err := crypto.Encrypt(dek, []byte(secret), []byte(projectID.String()))
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	id, err := s.repo.Create(ctx, projectID, name, req.URL, enc, events)
	if err != nil {
		return nil, err
	}
	s.audit.Record(ctx, a, p.OrgID, &projectID, "webhook.create", name, nil)
	return &Created{ID: id, Secret: secret}, nil
}

func (s *Service) Update(ctx context.Context, a principal.Actor, projectID, id uuid.UUID, req *UpdateRequest) error {
	p, err := s.access.Project(ctx, a.UserID, projectID, access.RoleAdmin)
	if err != nil {
		return err
	}
	row, err := s.repo.Get(ctx, projectID, id)
	if err != nil {
		return err
	}
	u := Update{Active: req.Active}
	if req.URL != nil {
		if err := s.validURL(*req.URL); err != nil {
			return err
		}
		u.URL = req.URL
	}
	if req.Events != nil {
		ev, ok := normEvents(*req.Events)
		if !ok {
			return ErrInvalidEvents
		}
		u.Events = &ev
	}
	name := row.Name
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		n := strings.TrimSpace(*req.Name)
		u.Name, name = &n, n
	}
	if err := s.repo.Update(ctx, id, u); err != nil {
		return err
	}
	s.audit.Record(ctx, a, p.OrgID, &projectID, "webhook.update", name, nil)
	return nil
}

func (s *Service) Delete(ctx context.Context, a principal.Actor, projectID, id uuid.UUID) error {
	p, err := s.access.Project(ctx, a.UserID, projectID, access.RoleAdmin)
	if err != nil {
		return err
	}
	row, err := s.repo.Get(ctx, projectID, id)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.audit.Record(ctx, a, p.OrgID, &projectID, "webhook.delete", row.Name, nil)
	return nil
}

func (s *Service) Test(ctx context.Context, a principal.Actor, projectID, id uuid.UUID) (*TestResult, error) {
	p, err := s.access.Project(ctx, a.UserID, projectID, access.RoleAdmin)
	if err != nil {
		return nil, err
	}
	row, err := s.repo.Get(ctx, projectID, id)
	if err != nil {
		return nil, err
	}
	dek, err := s.keys.ProjectKey(ctx, projectID)
	if err != nil {
		return nil, err
	}
	secret, err := crypto.Decrypt(dek, row.SecretEnc, []byte(projectID.String()))
	if err != nil {
		return nil, err
	}
	h := Hook{ID: id, ProjectID: projectID, URL: row.URL, Secret: string(secret), Events: row.Events}
	ev := audit.Event{ID: uuid.New(), OrgID: p.OrgID, ProjectID: &projectID, ActorType: "user", ActorLabel: a.Email, Action: "webhook.test", Resource: "ping", At: time.Now().UTC()}
	res := s.dispatcher.Deliver(ctx, h, ev, p.Name, false)
	s.repo.RecordDelivery(ctx, id, ev, res)
	return &TestResult{Status: res.Status, Error: res.Error, DurationMS: res.Duration.Milliseconds()}, nil
}

func (s *Service) Deliveries(ctx context.Context, a principal.Actor, projectID, id uuid.UUID) ([]Delivery, error) {
	if _, err := s.access.Project(ctx, a.UserID, projectID, access.RoleAdmin); err != nil {
		return nil, err
	}
	if _, err := s.repo.Get(ctx, projectID, id); err != nil {
		return nil, err
	}
	return s.repo.Deliveries(ctx, id)
}
