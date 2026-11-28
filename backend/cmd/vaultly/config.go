package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Credentials live in the user's config dir (never in the project), one profile per token:
//
//	~/.config/vaultly/credentials.json   {"profiles": {"default": {"url": "...", "token": "vst_..."}}}
//
// A project folder gets a .vaultly.json that only names the profile and path, so it is safe
// to commit and teammates each log in with their own token.

const projectFile = ".vaultly.json"

type Profile struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

type Credentials struct {
	Profiles map[string]Profile `json:"profiles"`
}

type ProjectConfig struct {
	Profile string `json:"profile,omitempty"`
	Path    string `json:"path,omitempty"`
}

func configDir() (string, error) {
	if d := os.Getenv("VAULTLY_CONFIG_DIR"); d != "" {
		return d, nil
	}
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "vaultly"), nil
}

func credentialsPath() (string, error) {
	d, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "credentials.json"), nil
}

func loadCredentials() (*Credentials, error) {
	c := &Credentials{Profiles: map[string]Profile{}}
	p, err := credentialsPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, err
	}
	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
	return c, nil
}

func saveCredentials(c *Credentials) error {
	p, err := credentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return writeFileAtomic(p, append(b, '\n'), 0o600)
}

// findProject walks up from dir looking for .vaultly.json, like git does for .git.
func findProject(dir string) (*ProjectConfig, string, error) {
	for {
		p := filepath.Join(dir, projectFile)
		b, err := os.ReadFile(p)
		if err == nil {
			pc := &ProjectConfig{}
			if err := json.Unmarshal(b, pc); err != nil {
				return nil, p, err
			}
			return pc, p, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, p, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, "", nil
		}
		dir = parent
	}
}

// writeFileAtomic writes to a temp file and renames it, so a crash never leaves half a file.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// settings is the resolved configuration for one command.
type settings struct {
	URL, Token, Path, Profile string
	Source                    string // where the token came from, for whoami
}

// resolve applies precedence: flags > environment > .vaultly.json > profile > defaults.
func resolve(flagProfile, flagURL, flagToken, flagPath string) (*settings, error) {
	s := &settings{Profile: "default"}
	cwd, _ := os.Getwd()
	pc, pcPath, err := findProject(cwd)
	if err != nil {
		return nil, err
	}
	if pc != nil {
		if pc.Profile != "" {
			s.Profile = pc.Profile
		}
		s.Path = pc.Path
	}
	if flagProfile != "" {
		s.Profile = flagProfile
	}
	creds, err := loadCredentials()
	if err != nil {
		return nil, err
	}
	if p, ok := creds.Profiles[s.Profile]; ok {
		s.URL, s.Token, s.Source = p.URL, p.Token, "profile "+s.Profile
		if pcPath != "" {
			s.Source += " (via " + pcPath + ")"
		}
	}
	if v := os.Getenv("VAULTLY_URL"); v != "" {
		s.URL = v
	}
	if v := os.Getenv("VAULTLY_TOKEN"); v != "" {
		s.Token, s.Source = v, "VAULTLY_TOKEN"
	}
	if flagURL != "" {
		s.URL = flagURL
	}
	if flagToken != "" {
		s.Token, s.Source = flagToken, "--token"
	}
	if flagPath != "" {
		s.Path = flagPath
	}
	if s.URL == "" {
		s.URL = "http://localhost:8080"
	}
	return s, nil
}
