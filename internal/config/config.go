package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	DefaultBasePath = "/mcp"
	DefaultTimeout  = 60 * time.Second
)

type File struct {
	MCPServers map[string]Server `json:"mcpServers"`
}

type Server struct {
	Enabled     bool              `json:"enabled"`
	Command     string            `json:"command"`
	Args        []string          `json:"args"`
	Env         map[string]string `json:"env"`
	Description string            `json:"description"`
	TimeoutMS   int               `json:"timeout"`
	Path        string            `json:"path,omitempty"`
}

type Runtime struct {
	SourcePath string
	BasePath   string
	Servers    []Route
}

type Route struct {
	Name        string
	Path        string
	Command     string
	Args        []string
	Env         map[string]string
	Description string
	Timeout     time.Duration
}

func Load(path string, basePath string) (*Runtime, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	return Parse(data, path, basePath)
}

func Parse(data []byte, sourcePath string, basePath string) (*Runtime, error) {
	var cfg File
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return cfg.Normalize(sourcePath, basePath)
}

func Format(data []byte) ([]byte, error) {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	formatted, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("format config: %w", err)
	}
	formatted = append(formatted, '\n')
	return formatted, nil
}

func (f File) Normalize(sourcePath string, basePath string) (*Runtime, error) {
	basePath = normalizePath(defaultString(basePath, DefaultBasePath))

	names := make([]string, 0, len(f.MCPServers))
	for name := range f.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)

	routes := make([]Route, 0, len(names))
	usedPaths := make(map[string]string, len(names))

	for _, name := range names {
		server := f.MCPServers[name]
		if !server.Enabled {
			continue
		}
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("config contains an empty server name")
		}
		if strings.TrimSpace(server.Command) == "" {
			return nil, fmt.Errorf("server %q is missing command", name)
		}

		routePath := server.Path
		if strings.TrimSpace(routePath) == "" {
			routePath = joinRoutePath(basePath, uniqueSlug(name, usedPaths, basePath))
		} else {
			routePath = normalizePath(routePath)
			if err := validateRoutePath(routePath); err != nil {
				return nil, fmt.Errorf("server %q has invalid path: %w", name, err)
			}
			if owner, exists := usedPaths[routePath]; exists {
				return nil, fmt.Errorf("route path %q is used by both %q and %q", routePath, owner, name)
			}
			usedPaths[routePath] = name
		}
		if err := validateRoutePath(routePath); err != nil {
			return nil, fmt.Errorf("server %q has invalid path: %w", name, err)
		}

		timeout := DefaultTimeout
		if server.TimeoutMS > 0 {
			timeout = time.Duration(server.TimeoutMS) * time.Millisecond
		}

		routes = append(routes, Route{
			Name:        name,
			Path:        routePath,
			Command:     server.Command,
			Args:        append([]string(nil), server.Args...),
			Env:         cloneEnv(server.Env),
			Description: server.Description,
			Timeout:     timeout,
		})
	}

	absSource, err := filepath.Abs(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	return &Runtime{
		SourcePath: absSource,
		BasePath:   basePath,
		Servers:    routes,
	}, nil
}

func (r Route) EnvPairs() []string {
	if len(r.Env) == 0 {
		return nil
	}

	keys := make([]string, 0, len(r.Env))
	for key := range r.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, key+"="+r.Env[key])
	}
	return pairs
}

func (r Route) Equal(other Route) bool {
	return r.Name == other.Name &&
		r.Path == other.Path &&
		r.Command == other.Command &&
		reflect.DeepEqual(r.Args, other.Args) &&
		reflect.DeepEqual(r.Env, other.Env) &&
		r.Description == other.Description &&
		r.Timeout == other.Timeout
}

func cloneEnv(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	path = "/" + strings.Trim(path, "/")
	if path == "//" {
		return "/"
	}
	return path
}

func joinRoutePath(basePath string, slug string) string {
	return normalizePath(strings.TrimRight(basePath, "/") + "/" + slug)
}

func validateRoutePath(path string) error {
	switch {
	case path == "/":
		return fmt.Errorf("root path is reserved")
	case path == "/healthz":
		return fmt.Errorf("/healthz is reserved")
	case path == "/_admin":
		return fmt.Errorf("/_admin is reserved")
	case strings.HasPrefix(path, "/_admin/"):
		return fmt.Errorf("/_admin/* is reserved")
	default:
		return nil
	}
}

func uniqueSlug(name string, usedPaths map[string]string, basePath string) string {
	baseSlug := slugify(name)
	slug := baseSlug
	for index := 2; ; index++ {
		path := joinRoutePath(basePath, slug)
		if _, exists := usedPaths[path]; !exists {
			usedPaths[path] = name
			return slug
		}
		slug = fmt.Sprintf("%s-%d", baseSlug, index)
	}
}

func slugify(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "server"
	}

	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			builder.WriteRune(r)
			lastDash = false
		case !lastDash:
			builder.WriteByte('-')
			lastDash = true
		}
	}

	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		return "server"
	}
	return slug
}
