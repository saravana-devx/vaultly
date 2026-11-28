// vaultly: CLI for apps, scripts and CI, using a service token.
//
//	vaultly login                     # paste a service token once; stored in ~/.config/vaultly (0600)
//	vaultly setup --path /app         # link this folder: writes .vaultly.json (no secrets, safe to commit)
//	vaultly run -- node server.js     # inject secrets; falls back to an encrypted cache if the server is down
//	vaultly pull > .env
//	vaultly set DB_PASSWORD s3cret
//
// VAULTLY_TOKEN and VAULTLY_URL override the stored profile (handy in CI).
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/term"

	"vaultly/internal/envfmt"
)

const defaultMaxAge = 24 * time.Hour

type opts struct {
	profile, url, token, path, format string
	maxAge                            time.Duration
	noCache                           bool
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	fs.Usage = usage
	var o opts
	fs.StringVar(&o.profile, "profile", "", "credentials profile")
	fs.StringVar(&o.url, "url", "", "server URL")
	fs.StringVar(&o.token, "token", "", "service token (prefer `vaultly login` or VAULTLY_TOKEN)")
	fs.StringVar(&o.path, "path", "", "folder path inside the token's scope")
	fs.StringVar(&o.format, "format", "dotenv", "dotenv | shell | json")
	fs.DurationVar(&o.maxAge, "max-age", defaultMaxAge, "oldest cached secrets to use when offline (0 = never use the cache)")
	fs.BoolVar(&o.noCache, "no-cache", false, "don't read or write the offline cache")
	must(fs.Parse(os.Args[2:]))
	rest := fs.Args()

	switch cmd {
	case "login":
		must(login(o))
	case "logout":
		must(logout(o))
	case "setup":
		must(setup(o))
	case "whoami":
		must(showWhoami(o))
	case "pull":
		must(pull(o))
	case "run":
		if len(rest) == 0 {
			usage()
		}
		must(run(o, rest))
	case "set":
		if len(rest) != 2 {
			usage()
		}
		must(set(o, rest[0], rest[1]))
	case "help", "-h", "--help":
		usage()
	default:
		usage()
	}
}

func login(o opts) error {
	tok := strings.TrimSpace(o.token)
	if tok == "" {
		var err error
		if tok, err = readToken(); err != nil {
			return err
		}
	}
	if !strings.HasPrefix(tok, "vst_") {
		return errors.New("that doesn't look like a service token (they start with vst_)")
	}
	s, err := resolve(o.profile, o.url, tok, "")
	if err != nil {
		return err
	}
	w, err := newClient(s).whoami()
	if err != nil {
		return fmt.Errorf("could not verify the token: %w", err)
	}
	creds, err := loadCredentials()
	if err != nil {
		return err
	}
	creds.Profiles[s.Profile] = Profile{URL: s.URL, Token: tok}
	if err := saveCredentials(creds); err != nil {
		return err
	}
	p, _ := credentialsPath()
	fmt.Fprintf(os.Stderr, "Logged in as %q: %s, %s, %s.\nSaved to %s (profile %q).\n", w.Name, w.Env, w.Path, access(w), p, s.Profile)
	return nil
}

// readToken prompts without echo on a terminal, or reads one line from a pipe.
func readToken() (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "Service token (input hidden): ")
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		return strings.TrimSpace(string(b)), err
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func logout(o opts) error {
	s, err := resolve(o.profile, "", "", "")
	if err != nil {
		return err
	}
	creds, err := loadCredentials()
	if err != nil {
		return err
	}
	if _, ok := creds.Profiles[s.Profile]; !ok {
		return fmt.Errorf("no profile %q", s.Profile)
	}
	delete(creds.Profiles, s.Profile)
	if err := saveCredentials(creds); err != nil {
		return err
	}
	if d, err := cacheDir(); err == nil {
		_ = os.RemoveAll(d) // cache entries are per token; dropping them all is simplest and safest
	}
	fmt.Fprintf(os.Stderr, "Logged out of profile %q and cleared the offline cache.\n", s.Profile)
	return nil
}

func setup(o opts) error {
	creds, err := loadCredentials()
	if err != nil {
		return err
	}
	profile := o.profile
	if profile == "" {
		profile = "default"
	}
	if _, ok := creds.Profiles[profile]; !ok {
		return fmt.Errorf("no profile %q yet: run `vaultly login --profile %s` first", profile, profile)
	}
	pc := ProjectConfig{Profile: profile, Path: o.path}
	if profile == "default" {
		pc.Profile = ""
	}
	b, _ := json.MarshalIndent(pc, "", "  ")
	cwd, _ := os.Getwd()
	p := filepath.Join(cwd, projectFile)
	if err := writeFileAtomic(p, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Wrote %s. Commands run in this folder now use profile %q", p, profile)
	if o.path != "" {
		fmt.Fprintf(os.Stderr, " and path %s", o.path)
	}
	fmt.Fprintln(os.Stderr, ". It holds no secrets, so it is safe to commit.")
	return nil
}

func showWhoami(o opts) error {
	s, err := resolve(o.profile, o.url, o.token, o.path)
	if err != nil {
		return err
	}
	w, err := newClient(s).whoami()
	if err != nil {
		return err
	}
	path := s.Path
	if path == "" {
		path = w.Path
	}
	fmt.Printf("token     %s\nscope     %s %s (%s)\nfetching  %s\nserver    %s\nfrom      %s\n", w.Name, w.Env, w.Path, access(w), path, s.URL, s.Source)
	return nil
}

func access(w *whoami) string {
	if w.CanWrite {
		return "read-write"
	}
	return "read-only"
}

// load resolves settings and fetches secrets, using the offline cache when allowed.
func load(o opts) (map[string]string, error) {
	s, err := resolve(o.profile, o.url, o.token, o.path)
	if err != nil {
		return nil, err
	}
	var ch *cache
	if !o.noCache {
		if d, err := cacheDir(); err == nil {
			ch, _ = newCache(d, s.URL, s.Token, s.Path)
		}
	}
	res, err := fetch(newClient(s), s.Path, ch, o.maxAge, time.Now)
	if err != nil {
		return nil, err
	}
	if res.Cached {
		fmt.Fprintf(os.Stderr, "vaultly: server unavailable (%v); using secrets cached %s ago\n", res.Cause, time.Since(res.CachedAt).Round(time.Second))
	}
	return res.Secrets, nil
}

func sortedPairs(m map[string]string) [][2]string {
	out := make([][2]string, 0, len(m))
	for k, v := range m {
		out = append(out, [2]string{k, v})
	}
	sort.Slice(out, func(a, b int) bool { return out[a][0] < out[b][0] })
	return out
}

func pull(o opts) error {
	m, err := load(o)
	if err != nil {
		return err
	}
	switch o.format {
	case "json":
		b, _ := json.MarshalIndent(map[string]any{"secrets": m}, "", "  ")
		fmt.Println(string(b))
	case "dotenv", "shell":
		fmt.Print(envfmt.Render(sortedPairs(m), o.format))
	default:
		return fmt.Errorf("unknown format %q", o.format)
	}
	return nil
}

func run(o opts, argv []string) error {
	m, err := load(o)
	if err != nil {
		return err
	}
	c := exec.Command(argv[0], argv[1:]...)
	c.Env = os.Environ()
	for _, kv := range sortedPairs(m) {
		c.Env = append(c.Env, kv[0]+"="+kv[1])
	}
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			os.Exit(ee.ExitCode())
		}
		return err
	}
	return nil
}

func set(o opts, key, value string) error {
	s, err := resolve(o.profile, o.url, o.token, o.path)
	if err != nil {
		return err
	}
	raw, err := newClient(s).do("PUT", "/api/v1/secrets", map[string]string{"key": key, "value": value, "path": s.Path})
	if err != nil {
		return err
	}
	var saved struct {
		Key     string `json:"key"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil {
		return err
	}
	fmt.Printf("saved %s (version %d)\n", saved.Key, saved.Version)
	return nil
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "vaultly:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: vaultly <command> [flags]

  login   [--profile p] [--url u] [--token t]   store a service token (prompted, hidden, if --token is omitted)
  setup   [--profile p] [--path /p]             link this folder via .vaultly.json (safe to commit)
  whoami                                        show the token's scope and where settings come from
  pull    [--format dotenv|shell|json]          print secrets
  run     -- <command...>                       run a command with secrets as environment variables
  set     KEY VALUE                             write a secret (write-enabled token)
  logout  [--profile p]                         forget a profile and clear the offline cache

flags for pull/run: --path /p  --max-age 24h (0 disables the offline cache)  --no-cache
env: VAULTLY_TOKEN, VAULTLY_URL override the stored profile; VAULTLY_CONFIG_DIR, VAULTLY_CACHE_DIR move the files.

Offline: every successful fetch is cached encrypted with a key derived from the token. If the server
can't be reached (or returns 5xx), pull/run use that copy and say so. A 401/403/404 never falls back.`)
	os.Exit(2)
}
