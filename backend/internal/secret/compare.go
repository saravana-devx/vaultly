package secret

import (
	"context"
	"crypto/sha256"
	"sort"
	"strings"

	"github.com/google/uuid"

	"vaultly/internal/pathutil"
	"vaultly/internal/principal"
)

// CompareRow is one (path, key) across environments. Values are never returned: Variant gives
// each environment a group number, and environments with equal values share a number, so the
// UI can say "prod differs" without anyone seeing either value.
type CompareRow struct {
	Path    string         `json:"path"`
	Key     string         `json:"key"`
	Variant map[string]int `json:"variant"` // env slug -> group; absent = missing in that env
	Status  string         `json:"status"`  // same | differs | missing
}

type Comparison struct {
	Envs    []string     `json:"envs"`
	Path    string       `json:"path"`
	Rows    []CompareRow `json:"rows"`
	Summary struct {
		Same    int `json:"same"`
		Differs int `json:"differs"`
		Missing int `json:"missing"`
	} `json:"summary"`
}

type envValues struct {
	env  string
	vals map[string]string // "path\x00key" -> plaintext
}

// compareEnvs groups values per row. A row is "missing" if any compared environment lacks it
// (that is the drift that breaks deploys), else "differs" if there is more than one variant.
func compareEnvs(order []string, data []envValues) []CompareRow {
	keys := map[string]bool{}
	for _, d := range data {
		for k := range d.vals {
			keys[k] = true
		}
	}
	rows := make([]CompareRow, 0, len(keys))
	for k := range keys {
		path, key, _ := strings.Cut(k, "\x00")
		row := CompareRow{Path: path, Key: key, Variant: map[string]int{}}
		groups := map[[32]byte]int{}
		for _, d := range data {
			v, ok := d.vals[k]
			if !ok {
				continue
			}
			h := sha256.Sum256([]byte(v)) // only ever compared in memory
			g, seen := groups[h]
			if !seen {
				g = len(groups)
				groups[h] = g
			}
			row.Variant[d.env] = g
		}
		switch {
		case len(row.Variant) < len(order):
			row.Status = "missing"
		case len(groups) > 1:
			row.Status = "differs"
		default:
			row.Status = "same"
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(a, b int) bool {
		if rows[a].Path != rows[b].Path {
			return rows[a].Path < rows[b].Path
		}
		return rows[a].Key < rows[b].Key
	})
	return rows
}

// Compare lines up every environment the caller can read (or the ones asked for) under path.
func (s *Service) Compare(ctx context.Context, a principal.Actor, projectID uuid.UUID, envs []string, rawPath string) (*Comparison, error) {
	sc, err := s.open(ctx, a, projectID)
	if err != nil {
		return nil, err
	}
	path, ok := pathutil.Norm(rawPath)
	if !ok {
		return nil, ErrInvalidPath
	}
	all, err := s.access.Envs(ctx, projectID)
	if err != nil {
		return nil, err
	}
	readable, err := s.access.ReadableEnvs(ctx, sc.p, a.UserID)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, e := range envs {
		want[e] = true
	}
	var order []string
	for _, e := range all { // keep the project's environment order
		if _, ok := readable[e.Slug]; ok && (len(want) == 0 || want[e.Slug]) {
			order = append(order, e.Slug)
		}
	}
	for e := range want {
		if _, ok := readable[e]; !ok {
			return nil, ErrCompareEnv
		}
	}
	if len(order) < 2 {
		return nil, ErrCompareTooFew
	}
	data := make([]envValues, 0, len(order))
	for _, env := range order {
		rows, err := s.repo.Live(ctx, readable[env], path, true)
		if err != nil {
			return nil, err
		}
		ev := envValues{env: env, vals: map[string]string{}}
		for _, r := range rows {
			v, err := s.decrypt(ctx, projectID, r.Ciphertext)
			if err != nil {
				return nil, err
			}
			ev.vals[r.Path+"\x00"+r.Key] = v
		}
		data = append(data, ev)
	}
	out := &Comparison{Envs: order, Path: path, Rows: compareEnvs(order, data)}
	for _, r := range out.Rows {
		switch r.Status {
		case "same":
			out.Summary.Same++
		case "differs":
			out.Summary.Differs++
		default:
			out.Summary.Missing++
		}
	}
	s.audit.Record(ctx, a, sc.p.OrgID, &projectID, "secret.compare", path, map[string]any{"envs": order, "keys": len(out.Rows)})
	return out, nil
}
