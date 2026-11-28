package secret

import (
	"strings"

	"vaultly/internal/envfmt"
)

func parseDotenv(content string) []Pair {
	var out []Pair
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch {
		case len(v) >= 2 && v[0] == '"' && strings.HasSuffix(v, `"`):
			v = strings.NewReplacer(`\n`, "\n", `\r`, "\r", `\"`, `"`, `\\`, `\`).Replace(v[1 : len(v)-1])
		case len(v) >= 2 && v[0] == '\'' && strings.HasSuffix(v, "'"):
			v = v[1 : len(v)-1]
		default:
			if i := strings.Index(v, " #"); i >= 0 {
				v = strings.TrimSpace(v[:i])
			}
		}
		out = append(out, Pair{k, v})
	}
	return out
}

// RenderText formats pairs as a .env file or as shell exports.
func RenderText(pairs []Pair, format string) string {
	out := make([][2]string, len(pairs))
	for i, p := range pairs {
		out[i] = p
	}
	return envfmt.Render(out, format)
}

func ToMap(pairs []Pair) map[string]string {
	m := make(map[string]string, len(pairs))
	for _, kv := range pairs {
		m[kv[0]] = kv[1]
	}
	return m
}
