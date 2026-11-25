package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/saravana/vaultly/internal/backup"
)

type exportVersion struct {
	Version        int       `json:"version"`
	EncryptedValue []byte    `json:"encrypted_value"`
	Action         string    `json:"action"`
	Actor          string    `json:"actor"`
	CreatedAt      time.Time `json:"created_at"`
}

type exportSecret struct {
	Env            string          `json:"env"`
	Path           string          `json:"path"`
	Key            string          `json:"key"`
	EncryptedValue []byte          `json:"encrypted_value"`
	Version        int             `json:"version"`
	Comment        string          `json:"comment"`
	Tags           []string        `json:"tags"`
	ExpiresAt      *time.Time      `json:"expires_at,omitempty"`
	UpdatedBy      string          `json:"updated_by"`
	UpdatedAt      time.Time       `json:"updated_at"`
	Versions       []exportVersion `json:"versions"`
}

type exportEnv struct {
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Position int    `json:"position"`
}

type exportProject struct {
	ID           uuid.UUID      `json:"id"`
	Name         string         `json:"name"`
	EncryptedDEK []byte         `json:"encrypted_dek"`
	CreatedAt    time.Time      `json:"created_at"`
	Environments []exportEnv    `json:"environments"`
	Secrets      []exportSecret `json:"secrets"`
}

type exportDoc struct {
	Format     int             `json:"format"`
	ExportedAt time.Time       `json:"exported_at"`
	OrgName    string          `json:"org_name"`
	Projects   []exportProject `json:"projects"`
}

// createBackup exports every project in the workspace as ciphertext and seals the
// file with a passphrase. Tokens, webhooks and integration credentials are left out on purpose.
func (s *Server) createBackup(c *gin.Context) {
	org, _, ok := s.orgRole(c, "admin")
	if !ok {
		return
	}
	var in struct{ Passphrase string }
	if c.ShouldBindJSON(&in) != nil || len(in.Passphrase) < 12 {
		fail(c, 400, "choose a passphrase of at least 12 characters")
		return
	}
	doc := exportDoc{Format: 1, ExportedAt: time.Now().UTC()}
	s.db.QueryRow(c, `SELECT name FROM orgs WHERE id=$1`, org).Scan(&doc.OrgName)

	prows, err := s.db.Query(c, `SELECT id,name,encrypted_dek,created_at FROM projects WHERE org_id=$1 ORDER BY name`, org)
	if err != nil {
		internalErr(c, err)
		return
	}
	for prows.Next() {
		var p exportProject
		if prows.Scan(&p.ID, &p.Name, &p.EncryptedDEK, &p.CreatedAt) == nil {
			doc.Projects = append(doc.Projects, p)
		}
	}
	prows.Close()

	total := 0
	for i := range doc.Projects {
		p := &doc.Projects[i]
		erows, err := s.db.Query(c, `SELECT name,slug,position FROM environments WHERE project_id=$1 ORDER BY position`, p.ID)
		if err != nil {
			internalErr(c, err)
			return
		}
		for erows.Next() {
			var e exportEnv
			if erows.Scan(&e.Name, &e.Slug, &e.Position) == nil {
				p.Environments = append(p.Environments, e)
			}
		}
		erows.Close()

		srows, err := s.db.Query(c, `SELECT s.id,e.slug,s.path,s.key,s.encrypted_value,s.version,s.comment,s.tags,s.expires_at,s.updated_by,s.updated_at
			FROM secrets s JOIN environments e ON e.id=s.env_id WHERE s.project_id=$1 ORDER BY e.position, s.path, s.key`, p.ID)
		if err != nil {
			internalErr(c, err)
			return
		}
		ids := map[uuid.UUID]int{}
		for srows.Next() {
			var id uuid.UUID
			var sec exportSecret
			if srows.Scan(&id, &sec.Env, &sec.Path, &sec.Key, &sec.EncryptedValue, &sec.Version, &sec.Comment, &sec.Tags, &sec.ExpiresAt, &sec.UpdatedBy, &sec.UpdatedAt) == nil {
				sec.UpdatedBy = s.openLabel(sec.UpdatedBy)
				ids[id] = len(p.Secrets)
				p.Secrets = append(p.Secrets, sec)
			}
		}
		srows.Close()
		vrows, err := s.db.Query(c, `SELECT v.secret_id,v.version,v.encrypted_value,v.action,v.actor,v.created_at FROM secret_versions v
			JOIN secrets s ON s.id=v.secret_id WHERE s.project_id=$1 ORDER BY v.secret_id, v.version`, p.ID)
		if err != nil {
			internalErr(c, err)
			return
		}
		for vrows.Next() {
			var sid uuid.UUID
			var v exportVersion
			if vrows.Scan(&sid, &v.Version, &v.EncryptedValue, &v.Action, &v.Actor, &v.CreatedAt) == nil {
				v.Actor = s.openLabel(v.Actor)
				if idx, ok := ids[sid]; ok {
					p.Secrets[idx].Versions = append(p.Secrets[idx].Versions, v)
				}
			}
		}
		vrows.Close()
		total += len(p.Secrets)
	}
	plain, err := json.Marshal(doc)
	if err != nil {
		internalErr(c, err)
		return
	}
	blob, err := backup.Seal(in.Passphrase, plain)
	if err != nil {
		internalErr(c, err)
		return
	}
	s.record(c, org, nil, "org.backup", doc.OrgName, map[string]any{"projects": len(doc.Projects), "secrets": total})
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="vaultly-backup-%s.vbk"`, time.Now().Format("2006-01-02")))
	c.Data(200, "application/octet-stream", blob)
}

// restoreBackup re-creates projects from a backup file under their original IDs
// (ciphertext is bound to the project ID). Existing projects are never overwritten.
func (s *Server) restoreBackup(c *gin.Context) {
	org, _, ok := s.orgRole(c, "admin")
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<20)
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		fail(c, 400, "attach the backup file")
		return
	}
	defer file.Close()
	blob, err := io.ReadAll(file)
	if err != nil {
		fail(c, 400, "could not read the file")
		return
	}
	plain, err := backup.Open(c.PostForm("passphrase"), blob)
	if err != nil {
		fail(c, 400, err.Error())
		return
	}
	var doc exportDoc
	if json.Unmarshal(plain, &doc) != nil || doc.Format != 1 {
		fail(c, 400, "unsupported backup format")
		return
	}
	type skip struct {
		Project string `json:"project"`
		Reason  string `json:"reason"`
	}
	restored, secretCount, skipped := 0, 0, []skip{}
	for _, p := range doc.Projects {
		var exists bool
		s.db.QueryRow(c, `SELECT EXISTS (SELECT 1 FROM projects WHERE id=$1)`, p.ID).Scan(&exists)
		if exists {
			skipped = append(skipped, skip{p.Name, "a project with this ID already exists"})
			continue
		}
		if _, err := s.keys.UnwrapDEK(c, p.EncryptedDEK); err != nil {
			skipped = append(skipped, skip{p.Name, "was encrypted with a different MASTER_KEY"})
			continue
		}
		err := pgx.BeginFunc(c, s.db, func(tx pgx.Tx) error {
			if _, err := tx.Exec(c, `INSERT INTO projects (id,org_id,name,encrypted_dek,created_at) VALUES ($1,$2,$3,$4,$5)`, p.ID, org, p.Name, p.EncryptedDEK, p.CreatedAt); err != nil {
				return err
			}
			envIDs := map[string]uuid.UUID{}
			for _, e := range p.Environments {
				var id uuid.UUID
				if err := tx.QueryRow(c, `INSERT INTO environments (project_id,name,slug,position) VALUES ($1,$2,$3,$4) RETURNING id`, p.ID, e.Name, e.Slug, e.Position).Scan(&id); err != nil {
					return err
				}
				envIDs[e.Slug] = id
			}
			for _, sec := range p.Secrets {
				eid, ok := envIDs[sec.Env]
				if !ok {
					continue
				}
				if sec.Tags == nil {
					sec.Tags = []string{}
				}
				var sid uuid.UUID
				if err := tx.QueryRow(c, `INSERT INTO secrets (project_id,env_id,path,key,encrypted_value,version,comment,tags,expires_at,updated_by,updated_at)
					VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
					p.ID, eid, sec.Path, sec.Key, sec.EncryptedValue, sec.Version, sec.Comment, sec.Tags, sec.ExpiresAt, s.sealLabel(sec.UpdatedBy), sec.UpdatedAt).Scan(&sid); err != nil {
					return err
				}
				for _, v := range sec.Versions {
					if _, err := tx.Exec(c, `INSERT INTO secret_versions (secret_id,version,encrypted_value,action,actor,created_at) VALUES ($1,$2,$3,$4,$5,$6)`,
						sid, v.Version, v.EncryptedValue, v.Action, s.sealLabel(v.Actor), v.CreatedAt); err != nil {
						return err
					}
				}
				secretCount++
			}
			return nil
		})
		if err != nil {
			skipped = append(skipped, skip{p.Name, "restore failed and was rolled back"})
			continue
		}
		restored++
	}
	s.record(c, org, nil, "org.restore", doc.OrgName, map[string]any{"restored": restored, "skipped": len(skipped), "secrets": secretCount})
	c.JSON(200, gin.H{"restored": restored, "secrets": secretCount, "skipped": skipped})
}
