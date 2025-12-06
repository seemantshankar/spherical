package startup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CLIBuilder handles building and checking CLI binaries.
type CLIBuilder struct {
	repoRoot string
	binDir   string
}

// NewCLIBuilder creates a new CLI builder.
func NewCLIBuilder(repoRoot, binDir string) *CLIBuilder {
	return &CLIBuilder{
		repoRoot: repoRoot,
		binDir:   binDir,
	}
}

// BinaryInfo represents information about a CLI binary.
type BinaryInfo struct {
	Name         string
	SourcePath   string
	BinaryPath   string
	NeedsRebuild bool
	Exists       bool
}

// CheckBinaries checks if CLI binaries exist and need rebuilding.
func (b *CLIBuilder) CheckBinaries(ctx context.Context) ([]BinaryInfo, error) {
	binaries := []BinaryInfo{
		{
			Name:       "pdf-extractor",
			SourcePath: filepath.Join(b.repoRoot, "libs/pdf-extractor/cmd/pdf-extractor"),
			BinaryPath: filepath.Join(b.binDir, "pdf-extractor"),
		},
		{
			Name:       "knowledge-engine-cli",
			SourcePath: filepath.Join(b.repoRoot, "libs/knowledge-engine/cmd/knowledge-engine-cli"),
			BinaryPath: filepath.Join(b.binDir, "knowledge-engine-cli"),
		},
	}
	
	// Ensure bin directory exists
	if err := os.MkdirAll(b.binDir, 0755); err != nil {
		return nil, fmt.Errorf("create bin directory: %w", err)
	}
	
	for i := range binaries {
		info := &binaries[i]
		
		// Check if binary exists
		if _, err := os.Stat(info.BinaryPath); err == nil {
			info.Exists = true
			
			// Check if source is newer than binary
			needsRebuild, err := b.needsRebuild(info.SourcePath, info.BinaryPath)
			if err != nil {
				// If we can't determine, assume rebuild is needed
				info.NeedsRebuild = true
			} else {
				info.NeedsRebuild = needsRebuild
			}
		} else {
			info.Exists = false
			info.NeedsRebuild = true
		}
	}
	
	return binaries, nil
}

// BuildBinary builds a single CLI binary.
func (b *CLIBuilder) BuildBinary(ctx context.Context, info BinaryInfo) error {
	// Determine the module path
	var modulePath string
	switch info.Name {
	case "pdf-extractor":
		modulePath = "github.com/spherical/pdf-extractor/cmd/pdf-extractor"
	case "knowledge-engine-cli":
		modulePath = "github.com/spherical-ai/spherical/libs/knowledge-engine/cmd/knowledge-engine-cli"
	default:
		return fmt.Errorf("unknown binary: %s", info.Name)
	}
	
	// Build command
	cmd := exec.CommandContext(ctx, "go", "build", "-o", info.BinaryPath, modulePath)
	cmd.Dir = b.repoRoot
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build %s: %w\nOutput: %s", info.Name, err, string(output))
	}
	
	return nil
}

// BuildAllNeeded builds all binaries that need rebuilding.
func (b *CLIBuilder) BuildAllNeeded(ctx context.Context, binaries []BinaryInfo) error {
	for _, info := range binaries {
		if info.NeedsRebuild {
			if err := b.BuildBinary(ctx, info); err != nil {
				return fmt.Errorf("build %s: %w", info.Name, err)
			}
		}
	}
	return nil
}

// needsRebuild checks if the source directory is newer than the binary.
func (b *CLIBuilder) needsRebuild(sourceDir, binaryPath string) (bool, error) {
	// Get binary modification time
	binaryInfo, err := os.Stat(binaryPath)
	if err != nil {
		return true, err
	}
	binaryTime := binaryInfo.ModTime()
	
	// Check all Go files in source directory
	needsRebuild, err := b.checkSourceFiles(sourceDir, binaryTime)
	if err != nil {
		return true, err
	}
	
	return needsRebuild, nil
}

// checkSourceFiles recursively checks if any Go files in the directory
// are newer than the binary.
func (b *CLIBuilder) checkSourceFiles(dir string, binaryTime time.Time) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		
		if entry.IsDir() {
			// Skip vendor, node_modules, etc.
			if entry.Name() == "vendor" || entry.Name() == "node_modules" {
				continue
			}
			
			needsRebuild, err := b.checkSourceFiles(path, binaryTime)
			if err != nil {
				return false, err
			}
			if needsRebuild {
				return true, nil
			}
		} else if strings.HasSuffix(entry.Name(), ".go") {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			
			if info.ModTime().After(binaryTime) {
				return true, nil
			}
		}
	}
	
	return false, nil
}

