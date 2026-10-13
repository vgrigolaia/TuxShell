// Package sshconfig discovers Host aliases in ~/.ssh/config and resolves
// their effective settings with `ssh -G`, so OpenSSH's own rules apply.
package sshconfig

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// maxIncludeDepth matches OpenSSH's READCONF_MAX_DEPTH.
const maxIncludeDepth = 16

// Entry is a concrete Host alias and the file that declares it.
type Entry struct {
	Alias string `json:"alias"`
	File  string `json:"file"`
}

// Aliases returns the concrete (non-wildcard) Host aliases declared in the
// config at path and any files it Includes, in declaration order.
// A missing config yields no aliases and no error.
func Aliases(path string) ([]Entry, error) {
	sshDir := filepath.Dir(path)
	seen := map[string]bool{}
	var out []Entry
	err := walk(path, sshDir, 0, seen, &out)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return out, err
}

func walk(path, sshDir string, depth int, seen map[string]bool, out *[]Entry) error {
	if depth > maxIncludeDepth {
		return fmt.Errorf("%s: Include nested too deeply", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, args := parseLine(sc.Text())
		switch key {
		case "host":
			for _, a := range args {
				if isConcrete(a) && !seen[a] {
					seen[a] = true
					*out = append(*out, Entry{Alias: a, File: path})
				}
			}
		case "include":
			for _, pattern := range args {
				pattern = expandHome(pattern)
				// Relative includes in the user config are relative to ~/.ssh.
				if !filepath.IsAbs(pattern) {
					pattern = filepath.Join(sshDir, pattern)
				}
				matches, _ := filepath.Glob(pattern)
				for _, m := range matches {
					if info, err := os.Stat(m); err == nil && info.Mode().IsRegular() {
						if err := walk(m, sshDir, depth+1, seen, out); err != nil && !os.IsNotExist(err) {
							return err
						}
					}
				}
			}
		}
	}
	return sc.Err()
}

// parseLine splits a config line into a lowercased keyword and its arguments,
// handling "Key=value", comments and double-quoted arguments.
func parseLine(line string) (string, []string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", nil
	}
	end := strings.IndexAny(line, " \t=")
	if end < 0 {
		return strings.ToLower(line), nil
	}
	key := strings.ToLower(line[:end])
	rest := strings.TrimLeft(line[end:], " \t")
	rest = strings.TrimPrefix(rest, "=")

	var args []string
	var cur strings.Builder
	inQuote, have := false, false
	for _, r := range rest {
		switch {
		case r == '"':
			inQuote = !inQuote
			have = true
		case (r == ' ' || r == '\t') && !inQuote:
			if have {
				args = append(args, cur.String())
				cur.Reset()
				have = false
			}
		case r == '#' && !inQuote && !have:
			return key, args
		default:
			cur.WriteRune(r)
			have = true
		}
	}
	if have {
		args = append(args, cur.String())
	}
	return key, args
}

func isConcrete(alias string) bool {
	return alias != "" && !strings.ContainsAny(alias, "*?!")
}

// Settings are the effective values ssh would use for an alias.
type Settings struct {
	Hostname  string `json:"hostname"`
	User      string `json:"user"`
	Port      int    `json:"port"`
	ProxyJump string `json:"proxyJump"`
}

// Resolve runs `ssh -G alias` and returns the settings that matter for display.
func Resolve(ctx context.Context, alias string) (Settings, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ssh", "-G", "--", alias).Output()
	if err != nil {
		return Settings{}, fmt.Errorf("ssh -G %s: %w", alias, err)
	}
	return parseG(string(out)), nil
}

func parseG(out string) Settings {
	var s Settings
	for line := range strings.Lines(out) {
		key, val, _ := strings.Cut(strings.TrimSpace(line), " ")
		switch key {
		case "hostname":
			s.Hostname = val
		case "user":
			s.User = val
		case "port":
			s.Port, _ = strconv.Atoi(val)
		case "proxyjump":
			if val != "none" {
				s.ProxyJump = val
			}
		}
	}
	return s
}

// DefaultPath returns ~/.ssh/config.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh", "config"), nil
}

func expandHome(path string) string {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, rest)
		}
	}
	return path
}
