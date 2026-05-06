package security

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrPathRequired         = errors.New("path is required")
	ErrPathEscapesWorkspace = errors.New("path escapes workspace")
)

type PathResolverOptions struct {
	DefaultWorkDir    string
	WorkspaceRoot     string
	DataMount         string
	AllowHostAbsolute bool
	AllowedRoots      []string
}

type PathResolver struct {
	defaultWorkDir    string
	workspaceRoot     string
	dataMount         string
	allowHostAbsolute bool
	allowedRoots      []string
}

func NewPathResolver(opts PathResolverOptions) (*PathResolver, error) {
	defaultWorkDir := cleanAbsOrPath(opts.DefaultWorkDir)
	if defaultWorkDir == "" {
		defaultWorkDir = "/data"
	}
	workspaceRoot := strings.TrimSpace(opts.WorkspaceRoot)
	if workspaceRoot == "" {
		workspaceRoot = defaultWorkDir
	}
	root, err := canonicalRoot(workspaceRoot)
	if err != nil {
		return nil, err
	}
	allowed := []string{root}
	for _, item := range opts.AllowedRoots {
		canonical, err := canonicalRoot(item)
		if err != nil {
			return nil, err
		}
		allowed = appendAllowedRoot(allowed, canonical)
	}
	dataMount := strings.TrimRight(strings.TrimSpace(opts.DataMount), string(filepath.Separator))
	if dataMount == "" {
		dataMount = "/data"
	}
	return &PathResolver{
		defaultWorkDir:    defaultWorkDir,
		workspaceRoot:     root,
		dataMount:         filepath.Clean(dataMount),
		allowHostAbsolute: opts.AllowHostAbsolute,
		allowedRoots:      allowed,
	}, nil
}

func (r *PathResolver) Resolve(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", ErrPathRequired
	}

	candidate, err := r.initialCandidate(path)
	if err != nil {
		return "", err
	}
	resolved, err := resolveExistingOrParent(candidate)
	if err != nil {
		return "", err
	}
	if !r.isAllowed(resolved) {
		return "", ErrPathEscapesWorkspace
	}
	return resolved, nil
}

func (r *PathResolver) DefaultWorkDir() string {
	return r.workspaceRoot
}

func (r *PathResolver) initialCandidate(path string) (string, error) {
	clean := filepath.Clean(path)
	if filepath.IsAbs(clean) {
		if r.isDataMountPath(clean) {
			rel := strings.TrimPrefix(clean, r.dataMount)
			return filepath.Join(r.workspaceRoot, strings.TrimPrefix(rel, string(filepath.Separator))), nil
		}
		if r.allowHostAbsolute {
			return clean, nil
		}
		return "", ErrPathEscapesWorkspace
	}
	return filepath.Join(r.workspaceRoot, clean), nil
}

func (r *PathResolver) isDataMountPath(path string) bool {
	return r.dataMount != "" && (path == r.dataMount || strings.HasPrefix(path, r.dataMount+string(filepath.Separator)))
}

func (r *PathResolver) isAllowed(path string) bool {
	for _, root := range r.allowedRoots {
		if path == root || strings.HasPrefix(path, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func cleanAbsOrPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func canonicalRoot(path string) (string, error) {
	path = cleanAbsOrPath(path)
	if path == "" {
		return "", ErrPathRequired
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved), nil
	}
	return path, nil
}

func resolveExistingOrParent(path string) (string, error) {
	path = cleanAbsOrPath(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved), nil
	}

	current := path
	var suffix []string
	for {
		if current == "." || current == string(filepath.Separator) || current == "" {
			return filepath.Clean(path), nil
		}
		if _, err := os.Lstat(current); err == nil {
			resolved, evalErr := filepath.EvalSymlinks(current)
			if evalErr != nil {
				return "", evalErr
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		suffix = append(suffix, filepath.Base(current))
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(path), nil
		}
		current = parent
	}
}

func appendAllowedRoot(roots []string, root string) []string {
	for _, existing := range roots {
		if existing == root {
			return roots
		}
	}
	return append(roots, root)
}
