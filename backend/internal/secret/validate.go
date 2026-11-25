package secret

import (
	"regexp"
	"strings"
)

const maxValueLen = 64 * 1024

var (
	keyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,127}$`)
	tagRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
)

func normTags(in []string) ([]string, bool) {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range in {
		t = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "#")))
		if t == "" || seen[t] {
			continue
		}
		if !tagRe.MatchString(t) {
			return nil, false
		}
		seen[t] = true
		out = append(out, t)
	}
	return out, len(out) <= 10
}
