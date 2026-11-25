// vaultly: CLI for fetching secrets with a service token.
//
//	export VAULTLY_TOKEN=vst_...  VAULTLY_URL=http://localhost:8080
//	vaultly pull > .env
//	vaultly run -- node server.js
//	vaultly set DB_PASSWORD s3cret
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmd, args := os.Args[1], os.Args[2:]
	path, format := "", "dotenv"
	var rest []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--path" && i+1 < len(args):
			path = args[i+1]
			i++
		case args[i] == "--format" && i+1 < len(args):
			format = args[i+1]
			i++
		case args[i] == "--":
			rest = append(rest, args[i+1:]...)
			i = len(args)
		default:
			rest = append(rest, args[i])
		}
	}
	switch cmd {
	case "pull":
		out := get(path, format, true)
		if format == "json" {
			out = data(out)
		}
		fmt.Print(string(out))
	case "run":
		if len(rest) == 0 {
			usage()
		}
		var m struct{ Secrets map[string]string }
		must(json.Unmarshal(data(get(path, "json", true)), &m))
		c := exec.Command(rest[0], rest[1:]...)
		c.Env = os.Environ()
		for k, v := range m.Secrets {
			c.Env = append(c.Env, k+"="+v)
		}
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := c.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				os.Exit(ee.ExitCode())
			}
			must(err)
		}
	case "set":
		if len(rest) != 2 {
			usage()
		}
		body, _ := json.Marshal(map[string]string{"key": rest[0], "value": rest[1], "path": path})
		req, _ := http.NewRequest("PUT", base()+"/api/v1/secrets", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		var saved struct {
			Key, Path string
			Version   int
		}
		must(json.Unmarshal(data(do(req)), &saved))
		fmt.Printf("saved %s (version %d)\n", saved.Key, saved.Version)
	default:
		usage()
	}
}

func base() string {
	if u := os.Getenv("VAULTLY_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://localhost:8080"
}

func get(path, format string, recursive bool) []byte {
	q := url.Values{"format": {format}}
	if path != "" {
		q.Set("path", path)
	}
	if recursive {
		q.Set("recursive", "true")
	}
	req, _ := http.NewRequest("GET", base()+"/api/v1/secrets?"+q.Encode(), nil)
	return do(req)
}

func do(req *http.Request) []byte {
	tok := os.Getenv("VAULTLY_TOKEN")
	if tok == "" {
		must(fmt.Errorf("VAULTLY_TOKEN is not set"))
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	must(err)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var e struct{ Message string }
		if json.Unmarshal(b, &e) != nil || e.Message == "" {
			e.Message = strings.TrimSpace(string(b))
		}
		must(fmt.Errorf("%s (HTTP %d)", e.Message, resp.StatusCode))
	}
	return b
}

// data returns the payload of the API's {success, message, data} envelope.
func data(b []byte) []byte {
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	must(json.Unmarshal(b, &env))
	return env.Data
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "vaultly:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  vaultly pull [--path /p] [--format dotenv|shell|json]   print secrets
  vaultly run [--path /p] -- <command...>                  run a command with secrets injected
  vaultly set [--path /p] KEY VALUE                        write a secret (write-enabled token)
env: VAULTLY_TOKEN (required), VAULTLY_URL (default http://localhost:8080)`)
	os.Exit(2)
}
