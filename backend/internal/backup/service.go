package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"vaultly/internal/access"
	"vaultly/internal/apperr"
	"vaultly/internal/audit"
	"vaultly/internal/keystore"
	"vaultly/internal/pii"
	"vaultly/internal/principal"
)

type BackupRepository interface {
	OrgName(ctx context.Context, org uuid.UUID) (string, error)
	Export(ctx context.Context, org uuid.UUID) ([]exportProject, error)
	ProjectExists(ctx context.Context, id uuid.UUID) (bool, error)
	Restore(ctx context.Context, org uuid.UUID, p exportProject) (int, error)
}

type Service struct {
	repo   BackupRepository
	access *access.Service
	keys   *keystore.Service
	pii    *pii.Box
	audit  *audit.Service
}

func NewService(repo BackupRepository, access *access.Service, keys *keystore.Service, box *pii.Box, audit *audit.Service) *Service {
	return &Service{repo: repo, access: access, keys: keys, pii: box, audit: audit}
}

// Create exports every project in the workspace as ciphertext and seals the file with a
// passphrase. Tokens, webhooks and integration credentials are left out on purpose.
func (s *Service) Create(ctx context.Context, a principal.Actor, org uuid.UUID, req *CreateRequest) (*File, error) {
	if _, err := s.access.Org(ctx, a.UserID, org, access.RoleAdmin); err != nil {
		return nil, err
	}
	if len(req.Passphrase) < 12 {
		return nil, ErrPassphrase
	}
	name, err := s.repo.OrgName(ctx, org)
	if err != nil {
		return nil, err
	}
	name = s.pii.OpenName("orgs.name", name)
	projects, err := s.repo.Export(ctx, org)
	if err != nil {
		return nil, err
	}
	total := 0
	for i := range projects {
		projects[i].Name = s.pii.OpenName("projects.name", projects[i].Name)
		for j := range projects[i].Secrets {
			sec := &projects[i].Secrets[j]
			sec.UpdatedBy = s.pii.OpenLabel(sec.UpdatedBy)
			for k := range sec.Versions {
				sec.Versions[k].Actor = s.pii.OpenLabel(sec.Versions[k].Actor)
			}
		}
		total += len(projects[i].Secrets)
	}
	plain, err := json.Marshal(exportDoc{Format: formatVersion, ExportedAt: time.Now().UTC(), OrgName: name, Projects: projects})
	if err != nil {
		return nil, err
	}
	blob, err := Seal(req.Passphrase, plain)
	if err != nil {
		return nil, err
	}
	s.audit.Record(ctx, a, org, nil, "org.backup", name, map[string]any{"projects": len(projects), "secrets": total})
	return &File{Name: fmt.Sprintf("vaultly-backup-%s.vbk", time.Now().Format("2006-01-02")), Data: blob}, nil
}

// Restore re-creates projects from a backup under their original IDs (ciphertext is bound to
// the project ID). Existing projects are never overwritten.
func (s *Service) Restore(ctx context.Context, a principal.Actor, org uuid.UUID, passphrase string, blob []byte) (*RestoreResult, error) {
	if _, err := s.access.Org(ctx, a.UserID, org, access.RoleAdmin); err != nil {
		return nil, err
	}
	plain, err := Open(passphrase, blob)
	if err != nil {
		return nil, apperr.Invalid(err.Error())
	}
	var doc exportDoc
	if json.Unmarshal(plain, &doc) != nil || doc.Format != formatVersion {
		return nil, ErrBadFormat
	}
	res := &RestoreResult{Skipped: []Skipped{}}
	for _, p := range doc.Projects {
		exists, err := s.repo.ProjectExists(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		if exists {
			res.Skipped = append(res.Skipped, Skipped{p.Name, ReasonExists})
			continue
		}
		if err := s.keys.Verify(ctx, p.EncryptedDEK); err != nil {
			res.Skipped = append(res.Skipped, Skipped{p.Name, ReasonWrongKey})
			continue
		}
		for i := range p.Secrets {
			sec := &p.Secrets[i]
			sec.UpdatedBy = s.pii.Label(sec.UpdatedBy)
			for k := range sec.Versions {
				sec.Versions[k].Actor = s.pii.Label(sec.Versions[k].Actor)
			}
		}
		restored := p
		restored.Name = s.pii.SealName("projects.name", p.Name)
		n, err := s.repo.Restore(ctx, org, restored)
		if err != nil {
			res.Skipped = append(res.Skipped, Skipped{p.Name, ReasonRollback})
			continue
		}
		res.Restored++
		res.Secrets += n
	}
	s.audit.Record(ctx, a, org, nil, "org.restore", doc.OrgName, map[string]any{"restored": res.Restored, "skipped": len(res.Skipped), "secrets": res.Secrets})
	return res, nil
}
