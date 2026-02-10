package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// CopyOptions configures project copy behavior.
type CopyOptions struct {
	Stdout io.Writer // For writing progress (defaults to os.Stdout)
}

// ProjectCopier handles copying project sessions to a target directory.
type ProjectCopier struct {
	registry *thinkt.StoreRegistry
	opts     CopyOptions
}

// NewProjectCopier creates a new project copier.
func NewProjectCopier(registry *thinkt.StoreRegistry, opts CopyOptions) *ProjectCopier {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	return &ProjectCopier{registry: registry, opts: opts}
}

// Copy copies all session files from a project to the target directory.
// projectPath is the original project path (e.g., /Users/evan/myproject).
// targetDir is where the files will be copied to.
func (c *ProjectCopier) Copy(projectQuery, targetDir string) error {
	project, err := c.findProject(projectQuery)
	if err != nil {
		return err
	}

	store, ok := c.registry.Get(project.Source)
	if !ok {
		return fmt.Errorf("source not available: %s", project.Source)
	}

	sessions, err := store.ListSessions(context.Background(), project.ID)
	if err != nil {
		return fmt.Errorf("list project sessions: %w", err)
	}
	if len(sessions) == 0 {
		return fmt.Errorf("no sessions found in %s", project.Path)
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("create target directory: %w", err)
	}

	copied, err := c.copyProjectFiles(sessions, targetDir)
	if err != nil {
		return err
	}

	fmt.Fprintf(c.opts.Stdout, "Copied %d files from %s to %s\n", copied, project.Path, targetDir)
	return nil
}

// findProject finds a project by its original path.
func (c *ProjectCopier) findProject(targetPath string) (*thinkt.Project, error) {
	project, err := ResolveProject(c.registry, targetPath)
	if err != nil {
		return nil, fmt.Errorf("%w\n\nUse 'thinkt projects list' to see available projects", err)
	}
	return project, nil
}

// copyProjectFiles copies all relevant files from source to target directory.
func (c *ProjectCopier) copyProjectFiles(sessions []thinkt.SessionMeta, dstDir string) (int, error) {
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].FullPath < sessions[j].FullPath
	})

	usedNames := make(map[string]struct{})
	copied := 0
	for _, session := range sessions {
		srcPath := strings.TrimSpace(session.FullPath)
		if srcPath == "" {
			continue
		}

		name := filepath.Base(srcPath)
		dstPath, err := nextAvailableTargetPath(dstDir, name, usedNames)
		if err != nil {
			return copied, err
		}

		if err := copyFile(srcPath, dstPath); err != nil {
			return copied, fmt.Errorf("copy %s: %w", srcPath, err)
		}
		copied++
	}

	if copied == 0 {
		return 0, fmt.Errorf("no session files found to copy")
	}

	return copied, nil
}

func nextAvailableTargetPath(dstDir, fileName string, used map[string]struct{}) (string, error) {
	name := fileName
	for i := 2; ; i++ {
		if _, exists := used[name]; !exists {
			candidate := filepath.Join(dstDir, name)
			_, err := os.Stat(candidate)
			if os.IsNotExist(err) {
				used[name] = struct{}{}
				return candidate, nil
			}
			if err != nil {
				return "", fmt.Errorf("check target file %s: %w", candidate, err)
			}
		}
		name = withCopySuffix(fileName, i)
	}
}

func withCopySuffix(fileName string, n int) string {
	ext := filepath.Ext(fileName)
	base := strings.TrimSuffix(fileName, ext)
	return fmt.Sprintf("%s_%d%s", base, n, ext)
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Get source file info for permissions
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
