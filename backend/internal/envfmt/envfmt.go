// Package envfmt renders KEY/VALUE pairs as a .env file or shell exports. It has no
// dependencies, so the server and the CLI share exactly the same output.
package envfmt

import (
	"fmt"
	"regexp"
	"strings"
)

var shellNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Quote returns v as a dotenv value, quoted and escaped only when it needs to be.
func Quote(v string) string {
	if v == "" || !strings.ContainsAny(v, " \t\n\r#\"'\\$`=") {
		return v
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`)
	return `"` + r.Replace(v) + `"`
}

// Render formats pairs as "dotenv" (default) or "shell".
func Render(pairs [][2]string, format string) string {
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
			fmt.Fprintf(&b, "%s=%s\n", kv[0], Quote(kv[1]))
		}
	}
	return b.String()
}
