package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Brain-STM-org/thinking-tracer-tools/internal/claude"
)

// CopyOptions configures project copy behavior.
type CopyOptions struct {
	Stdout io.Writer // For writing progress (defaults to os.Stdout)
}

// ProjectCopier handles copying project sessions to a target directory.
type ProjectCopier struct {
	baseDir string
	opts    CopyOptions
}

// NewProjectCopier creates a new project copier.
func NewProjectCopier(baseDir string, opts CopyOptions) *ProjectCopier {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	return &ProjectCopier{baseDir: baseDir, opts: opts}
}

// Copy copies all session files from a project to the target directory.
// projectPath is the original project path (e.g., /Users/evan/myproject).
// targetDir is where the files will be copied to.
func (c *ProjectCopier) Copy(projectPath, targetDir string) error {
	// Resolve to absolute path
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	// Find the project
	project, err := c.findProject(absPath)
	if err != nil {
		return err
	}

	// Create target directory
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("create target directory: %w", err)
	}

	// Copy files from project directory
	copied, err := c.copyProjectFiles(project.DirPath, targetDir)
	if err != nil {
		return err
	}

	fmt.Fprintf(c.opts.Stdout, "Copied %d files from %s to %s\n", copied, project.FullPath, targetDir)
	return nil
}

// findProject finds a project by its original path.
func (c *ProjectCopier) findProject(targetPath string) (*claude.Project, error) {
	projects, err := claude.ListProjects(c.baseDir)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	for _, p := range projects {
		if p.FullPath == targetPath {
			return &p, nil
		}
	}

	// Check if directory exists but has no sessions
	encodedName := encodePathToDirName(targetPath)
	projectsDir, _ := claude.ProjectsDir(c.baseDir)
	potentialDir := filepath.Join(projectsDir, encodedName)
	if info, err := os.Stat(potentialDir); err == nil && info.IsDir() {
		return nil, fmt.Errorf("no sessions found in %s", targetPath)
	}

	return nil, fmt.Errorf("project not found: %s\n\nUse 'thinkt projects' to list available projects", targetPath)
}

// copyProjectFiles copies all relevant files from source to target directory.
func (c *ProjectCopier) copyProjectFiles(srcDir, dstDir string) (int, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return 0, fmt.Errorf("read source directory: %w", err)
	}

	copied := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Copy session files and index files
		if strings.HasSuffix(name, ".jsonl") || strings.HasSuffix(name, ".json") {
			srcPath := filepath.Join(srcDir, name)
			dstPath := filepath.Join(dstDir, name)

			if err := copyFile(srcPath, dstPath); err != nil {
				return copied, fmt.Errorf("copy %s: %w", name, err)
			}
			copied++
		}
	}

	return copied, nil
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
