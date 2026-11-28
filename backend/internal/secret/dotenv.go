package secret

import (
	"fmt"
	"regexp"
	"strings"
)

var shellNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

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

func dotenvQuote(v string) string {
	if v == "" || !strings.ContainsAny(v, " \t\n\r#\"'\\$`=") {
		return v
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`)
	return `"` + r.Replace(v) + `"`
}

// RenderText formats pairs as a .env file or as shell exports.
func RenderText(pairs []Pair, format string) string {
	var b strings.Builder
	for _, kv := range pairs {
		if format == "shell" {
			// Keys may contain '.' or '-', which no shell accepts in a variable name; one such
			// line would make `eval "$(...)"` fail, so it is left out with a note instead.
			if !shellNameRe.MatchString(kv[0]) {
				fmt.Fprintf(&b, "# skipped %s: not a valid shell variable name\n", kv[0])
				continue
			}
			fmt.Fprintf(&b, "export %s='%s'\n", kv[0], strings.ReplaceAll(kv[1], "'", `'\''`))
		} else {
			fmt.Fprintf(&b, "%s=%s\n", kv[0], dotenvQuote(kv[1]))
		}
	}
	return b.String()
}

func ToMap(pairs []Pair) map[string]string {
	m := make(map[string]string, len(pairs))
	for _, kv := range pairs {
		m[kv[0]] = kv[1]
	}
	return m
}
