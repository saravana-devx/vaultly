package secret

import (
	"encoding/json"
	"strings"
	"testing"
)

func vals(kv ...string) map[string]string {
	m := map[string]string{}
	for i := 0; i < len(kv); i += 2 {
		path, key, _ := strings.Cut(kv[i], ":")
		m[path+"\x00"+key] = kv[i+1]
	}
	return m
}

func TestCompareEnvs(t *testing.T) {
	order := []string{"dev", "staging", "prod"}
	data := []envValues{
		{"dev", vals("/:HOST", "db.local", "/:PORT", "5432", "/:DEBUG", "1", "/stripe:KEY", "sk_test")},
		{"staging", vals("/:HOST", "db.stage", "/:PORT", "5432", "/stripe:KEY", "sk_test")},
		{"prod", vals("/:HOST", "db.prod", "/:PORT", "5432", "/stripe:KEY", "sk_live")},
	}
	rows := compareEnvs(order, data)
	got := map[string]CompareRow{}
	for _, r := range rows {
		got[r.Path+":"+r.Key] = r
	}

	if r := got["/:PORT"]; r.Status != "same" || len(r.Variant) != 3 || r.Variant["dev"] != r.Variant["prod"] {
		t.Errorf("PORT: %+v", r)
	}
	if r := got["/:HOST"]; r.Status != "differs" || r.Variant["dev"] == r.Variant["staging"] || r.Variant["staging"] == r.Variant["prod"] {
		t.Errorf("HOST: every env differs: %+v", r)
	}
	if r := got["/stripe:KEY"]; r.Status != "differs" || r.Variant["dev"] != r.Variant["staging"] || r.Variant["dev"] == r.Variant["prod"] {
		t.Errorf("KEY: dev and staging share a value, prod differs: %+v", r)
	}
	if r := got["/:DEBUG"]; r.Status != "missing" || len(r.Variant) != 1 {
		t.Errorf("DEBUG: only in dev: %+v", r)
	}
	// Sorted by path, then key.
	if rows[0].Path != "/" || rows[0].Key != "DEBUG" || rows[len(rows)-1].Path != "/stripe" {
		t.Errorf("unexpected order: %+v", rows)
	}
}

func TestCompareNeverReturnsValues(t *testing.T) {
	data := []envValues{
		{"dev", vals("/:TOKEN", "super-secret-dev")},
		{"prod", vals("/:TOKEN", "super-secret-prod")},
	}
	b, _ := json.Marshal(compareEnvs([]string{"dev", "prod"}, data))
	if strings.Contains(string(b), "super-secret") {
		t.Fatalf("comparison leaked a value: %s", b)
	}
}

func TestCompareEmptyEnvs(t *testing.T) {
	rows := compareEnvs([]string{"dev", "prod"}, []envValues{{"dev", vals()}, {"prod", vals()}})
	if len(rows) != 0 {
		t.Fatalf("want no rows, got %+v", rows)
	}
}
