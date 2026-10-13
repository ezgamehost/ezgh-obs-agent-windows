// Package config validates configuration without including user-controlled values in errors.
package config

import (
	"errors"
	"github.com/ezgamehost/ezgh-obs-agent-windows/internal/platform"
	"go.yaml.in/yaml/v3"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	v, e := time.ParseDuration(n.Value)
	d.Duration = v
	return e
}

type Channel struct {
	ID         string `yaml:"id"`
	Name       string `yaml:"name"`
	Query      string `yaml:"query"`
	Service    string `yaml:"service"`
	StartAt    string `yaml:"start_at"`
	Generation int    `yaml:"generation"`
}
type File struct {
	ID       string   `yaml:"id"`
	Paths    []string `yaml:"paths"`
	Exclude  []string `yaml:"exclude"`
	Service  string   `yaml:"service"`
	StartAt  string   `yaml:"start_at"`
	Encoding string   `yaml:"encoding"`
}
type Config struct {
	Version        int    `yaml:"version"`
	Target         string `yaml:"target"`
	Endpoint       string `yaml:"endpoint"`
	Environment    string `yaml:"environment"`
	CredentialFile string `yaml:"credential_file"`
	StateDir       string `yaml:"state_dir"`
	CAFile         string `yaml:"ca_file"`
	HostID         string `yaml:"host_id"`
	Proxy          string `yaml:"proxy"`
	Metrics        struct {
		Enabled    bool     `yaml:"enabled"`
		Interval   Duration `yaml:"interval"`
		Collectors []string `yaml:"collectors"`
		MaxPoints  int      `yaml:"max_points"`
		MaxBytes   int      `yaml:"max_bytes"`
	} `yaml:"metrics"`
	Logs struct {
		EventLog struct {
			Channels []Channel `yaml:"channels"`
		} `yaml:"eventlog"`
		Files        []File `yaml:"files"`
		MaxOpenFiles int    `yaml:"max_open_files"`
	} `yaml:"logs"`
	Storage struct {
		LogBytes     int64  `yaml:"log_bytes"`
		MetricBytes  int64  `yaml:"metric_bytes"`
		ReserveBytes uint64 `yaml:"reserve_bytes"`
	} `yaml:"storage"`
}

var ident = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)
var keyRE = regexp.MustCompile(`^ezgh_ik_[A-Za-z0-9]{43}$`)
var DefaultCollectors = []string{"cpu", "memory", "logical_disk", "net", "os", "system"}

func LocalPath(s string) bool {
	return len(s) > 3 && ((s[0] >= 'A' && s[0] <= 'Z') || (s[0] >= 'a' && s[0] <= 'z')) && s[1] == ':' && (s[2] == '\\' || s[2] == '/') && !strings.ContainsAny(s[2:], ":\x00") && !strings.Contains(s, "..")
}
func Parse(r io.Reader) (Config, error) {
	var c Config
	c.Endpoint = "https://obs-ingest.ezghcloud.com"
	c.Metrics.Enabled = true
	c.Metrics.Interval.Duration = 30 * time.Second
	c.Metrics.Collectors = append([]string(nil), DefaultCollectors...)
	c.Metrics.MaxPoints = 50000
	c.Metrics.MaxBytes = 16 << 20
	c.Logs.MaxOpenFiles = 128
	c.Logs.EventLog.Channels = []Channel{{ID: "application", Name: "Application", StartAt: "end"}, {ID: "system", Name: "System", StartAt: "end"}}
	c.Storage.LogBytes = 256 << 20
	c.Storage.MetricBytes = 128 << 20
	c.Storage.ReserveBytes = 64 << 20
	b, e := io.ReadAll(io.LimitReader(r, (1<<20)+1))
	if e != nil || len(b) > 1<<20 {
		return c, errors.New("configuration exceeds size limit")
	}
	d := yaml.NewDecoder(strings.NewReader(string(b)))
	d.KnownFields(true)
	if d.Decode(&c) != nil {
		return c, errors.New("invalid configuration YAML")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return c, errors.New("expected one YAML document")
	}
	bad := func() (Config, error) {
		return c, errors.New("invalid configuration: check schema, identifiers, paths, sources and limits")
	}
	if c.Version != 1 || !ident.MatchString(c.Target) || !ident.MatchString(c.Environment) {
		return bad()
	}
	u, e := url.Parse(c.Endpoint)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return bad()
	}
	c.Endpoint = strings.TrimSuffix(c.Endpoint, "/")
	if !LocalPath(c.StateDir) || !LocalPath(c.CredentialFile) || (c.CAFile != "" && !LocalPath(c.CAFile)) {
		return bad()
	}
	if c.Proxy != "" {
		u, e = url.Parse(c.Proxy)
		if e != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
			return bad()
		}
	}
	if len(c.HostID) > 128 || strings.ContainsAny(c.HostID, "\x00\r\n") || strings.TrimSpace(c.HostID) != c.HostID {
		return bad()
	}
	if c.Metrics.Interval.Duration < time.Second || c.Metrics.Interval.Duration > 24*time.Hour || c.Metrics.MaxPoints < 1 || c.Metrics.MaxPoints > 1000000 || c.Metrics.MaxBytes < 1<<20 || c.Metrics.MaxBytes > 64<<20 {
		return bad()
	}
	seen := map[string]bool{}
	for _, v := range c.Metrics.Collectors {
		ok := false
		for _, a := range DefaultCollectors {
			ok = ok || v == a
		}
		if !ok || seen[v] {
			return bad()
		}
		seen[v] = true
	}
	if c.Metrics.Enabled && len(seen) == 0 {
		return bad()
	}
	ids := map[string]bool{}
	names := map[string]bool{}
	if len(c.Logs.EventLog.Channels) > 32 {
		return bad()
	}
	for i := range c.Logs.EventLog.Channels {
		v := &c.Logs.EventLog.Channels[i]
		name := strings.ToLower(v.Name)
		if !ident.MatchString(v.ID) || ids[v.ID] || names[name] || v.Name == "" || len(v.Name) > 256 || strings.ContainsAny(v.Name, "\x00\r\n") || len(v.Query) > 4096 || strings.ContainsRune(v.Query, 0) || v.Generation < 0 {
			return bad()
		}
		ids[v.ID] = true
		names[name] = true
		if v.StartAt == "" {
			v.StartAt = "end"
		}
		if v.StartAt != "end" && v.StartAt != "beginning" {
			return bad()
		}
		if v.Service == "" {
			v.Service = "windows-eventlog"
		}
		if len(v.Service) > 128 {
			return bad()
		}
	}
	paths := map[string]bool{}
	for i := range c.Logs.Files {
		v := &c.Logs.Files[i]
		if !ident.MatchString(v.ID) || ids[v.ID] || len(v.Paths) == 0 || v.Service == "" || len(v.Service) > 128 {
			return bad()
		}
		ids[v.ID] = true
		if v.StartAt == "" {
			v.StartAt = "end"
		}
		if v.Encoding == "" {
			v.Encoding = "auto"
		}
		if (v.StartAt != "end" && v.StartAt != "beginning") || (v.Encoding != "auto" && v.Encoding != "utf-8" && v.Encoding != "utf-16le") {
			return bad()
		}
		for _, p := range append(append([]string{}, v.Paths...), v.Exclude...) {
			if !LocalPath(p) || strings.Contains(p, "**") || strings.ContainsAny(p, "[]") {
				return bad()
			}
		}
		for _, p := range v.Paths {
			p = strings.ToLower(p)
			if paths[p] {
				return bad()
			}
			paths[p] = true
		}
	}
	if c.Logs.MaxOpenFiles < 1 || c.Logs.MaxOpenFiles > 4096 || (!c.Metrics.Enabled && len(c.Logs.Files)+len(c.Logs.EventLog.Channels) == 0) {
		return bad()
	}
	if c.Storage.LogBytes < 8<<20 || c.Storage.MetricBytes < 8<<20 || c.Storage.LogBytes > 1<<40 || c.Storage.MetricBytes > 1<<40 || c.Storage.ReserveBytes > 1<<40 {
		return bad()
	}
	return c, nil
}
func Load(path string) (Config, error) {
	f, e := platform.OpenConfig(path)
	if e != nil {
		return Config{}, errors.New("cannot securely open configuration")
	}
	defer f.Close()
	return Parse(f)
}
func ReadCredential(path string) (string, error) {
	f, e := platform.OpenSecret(path)
	if e != nil {
		return "", errors.New("credential file access or security check failed")
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 257))
	v := strings.TrimSpace(string(b))
	if e != nil || len(b) > 256 || !keyRE.MatchString(v) {
		return "", errors.New("invalid project ingest key format")
	}
	return v, nil
}

var _ = os.ErrNotExist
