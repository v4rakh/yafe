package engine

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

// ToolDeclaration defines a tool requirement in the flow.
type ToolDeclaration struct {
	Name    string `yaml:"name"`
	URL     string `yaml:"url"`
	Path    string `yaml:"path"`
	SHA256  string `yaml:"sha256"`
	Retries int    `yaml:"retries"`
}

// ToolMeta stores metadata for cache invalidation.
type ToolMeta struct {
	URL     string `json:"url"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Retries int    `json:"retries"`
}

// ToolManager handles tool resolution and caching.
type ToolManager struct {
	toolsDir string
}

// NewToolManager creates a new tool manager.
func NewToolManager(toolsDir string) *ToolManager {
	return &ToolManager{toolsDir: toolsDir}
}

// Initialize creates the tools' directory.
func (m *ToolManager) Initialize() error {
	if err := os.MkdirAll(m.toolsDir, dirMode); err != nil {
		return fmt.Errorf("creating tools directory %s: %w", m.toolsDir, err)
	}
	log.Debug().Msgf("Initialized tools directory at %s", m.toolsDir)
	return nil
}

// ToolsDir returns the path to the tools' directory.
func (m *ToolManager) ToolsDir() string {
	return m.toolsDir
}

// Resolve resolves all tool declarations.
func (m *ToolManager) Resolve(declarations []ToolDeclaration) error {
	for _, decl := range declarations {
		if decl.URL == "" {
			if err := m.resolvePathTool(decl); err != nil {
				return err
			}
		} else {
			if err := m.resolveDownloadedTool(decl); err != nil {
				return err
			}
		}
	}
	return nil
}

// resolvePathTool verifies a tool exists in PATH.
func (m *ToolManager) resolvePathTool(decl ToolDeclaration) error {
	path, err := exec.LookPath(decl.Name)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrToolNotFound, decl.Name)
	}
	log.Debug().Msgf("Tool %s found in PATH at %s", decl.Name, path)
	return nil
}

// resolveDownloadedTool downloads or uses cached tool.
func (m *ToolManager) resolveDownloadedTool(decl ToolDeclaration) error {
	toolPath := filepath.Join(m.toolsDir, decl.Name)

	needsDownload, err := m.needsDownload(decl)
	if err != nil {
		return err
	}

	if !needsDownload {
		log.Debug().Msgf("Tool %s already cached at %s", decl.Name, toolPath)
		return nil
	}

	log.Debug().Msgf("Downloading tool %s from %s", decl.Name, decl.URL)

	retries := decl.Retries
	if retries < 1 {
		retries = defaultRetries
	}

	if isArchiveURL(decl.URL) {
		// Download archive to temp file
		tmpFile, err := os.CreateTemp("", "yafe-tool-archive-*")
		if err != nil {
			return fmt.Errorf("creating temp file for archive: %w", err)
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()
		defer os.Remove(tmpPath)

		if err := m.download(decl.URL, tmpPath, retries); err != nil {
			return err
		}

		if decl.SHA256 != "" {
			if err := m.verifyChecksum(tmpPath, decl.SHA256); err != nil {
				return err
			}
		}

		if err := m.extractArchiveByURL(tmpPath, decl.URL, m.toolsDir, decl.Name, decl.Path); err != nil {
			return err
		}
	} else {
		// Direct binary download
		if err := m.download(decl.URL, toolPath, retries); err != nil {
			return err
		}

		if decl.SHA256 != "" {
			if err := m.verifyChecksum(toolPath, decl.SHA256); err != nil {
				os.Remove(toolPath)
				return err
			}
		}

		if err := os.Chmod(toolPath, scriptFileMode); err != nil {
			return fmt.Errorf("making tool %s executable: %w", decl.Name, err)
		}
	}

	if err := m.saveMeta(decl); err != nil {
		return err
	}

	log.Debug().Msgf("Tool %s installed at %s", decl.Name, toolPath)
	return nil
}

// needsDownload checks if tool needs to be downloaded based on meta comparison.
func (m *ToolManager) needsDownload(decl ToolDeclaration) (bool, error) {
	toolPath := filepath.Join(m.toolsDir, decl.Name)

	// Check if binary exists
	if _, err := os.Stat(toolPath); os.IsNotExist(err) {
		return true, nil
	} else if err != nil {
		return false, fmt.Errorf("checking tool %s: %w", decl.Name, err)
	}

	// Load existing meta
	meta, err := m.loadMeta(decl.Name)
	if err != nil {
		// No meta or error reading - need to download
		return true, nil
	}

	// Compare meta with declaration
	if meta.URL != decl.URL || meta.Path != decl.Path || meta.SHA256 != decl.SHA256 {
		return true, nil
	}

	return false, nil
}

// download fetches URL to destination with retry support.
func (m *ToolManager) download(url, dest string, retries int) error {
	var lastErr error

	for attempt := 1; attempt <= retries; attempt++ {
		if attempt > 1 {
			log.Debug().Msgf("Download retry %d/%d for %s", attempt, retries, url)
		}

		err := m.doDownload(url, dest)
		if err == nil {
			return nil
		}

		lastErr = err
		log.Debug().Msgf("Download attempt %d failed: %v", attempt, err)
	}

	return fmt.Errorf("%w: %s: %w", ErrToolDownload, url, lastErr)
}

// doDownload performs a single download attempt.
func (m *ToolManager) doDownload(url, dest string) error {
	//gosec:disable G304 -- URL comes from flow definition
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	//gosec:disable G304 -- Path is constructed from state dir + tool name
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// verifyChecksum validates file against expected SHA256.
func (m *ToolManager) verifyChecksum(path, expected string) error {
	//gosec:disable G304 -- Path is constructed from state dir + tool name
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening file for checksum: %w", err)
	}
	defer file.Close()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return fmt.Errorf("computing checksum: %w", err)
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("%w: expected %s, got %s", ErrToolChecksum, expected, actual)
	}

	log.Debug().Msgf("Checksum verified for %s", path)
	return nil
}

// extractArchiveByURL extracts a binary from an archive, using URL to detect format.
func (m *ToolManager) extractArchiveByURL(archive, url, destDir, toolName, binaryPath string) error {
	lowerURL := strings.ToLower(url)

	var err error
	switch {
	case strings.HasSuffix(lowerURL, ".tar.gz") || strings.HasSuffix(lowerURL, ".tgz"):
		err = m.extractTarGz(archive, destDir, toolName, binaryPath)
	case strings.HasSuffix(lowerURL, ".tar.bz2"):
		err = m.extractTarBz2(archive, destDir, toolName, binaryPath)
	case strings.HasSuffix(lowerURL, ".tar"):
		err = m.extractTar(archive, destDir, toolName, binaryPath)
	case strings.HasSuffix(lowerURL, ".zip"):
		err = m.extractZip(archive, destDir, toolName, binaryPath)
	default:
		return fmt.Errorf("%w: unsupported archive format", ErrToolExtract)
	}

	if err != nil {
		return err
	}

	// Make binary executable
	toolPath := filepath.Join(destDir, toolName)
	if err := os.Chmod(toolPath, scriptFileMode); err != nil {
		return fmt.Errorf("making extracted tool %s executable: %w", toolName, err)
	}

	return nil
}

// extractTarGz extracts a gzipped tar archive.
func (m *ToolManager) extractTarGz(archive, destDir, toolName, binaryPath string) error {
	//gosec:disable G304 -- archive is temp file path
	file, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("%w: opening archive: %w", ErrToolExtract, err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("%w: creating gzip reader: %w", ErrToolExtract, err)
	}
	defer gzr.Close()

	return m.extractFromTar(tar.NewReader(gzr), destDir, toolName, binaryPath)
}

// extractTarBz2 extracts a bzip2 tar archive.
func (m *ToolManager) extractTarBz2(archive, destDir, toolName, binaryPath string) error {
	//gosec:disable G304 -- archive is temp file path
	file, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("%w: opening archive: %w", ErrToolExtract, err)
	}
	defer file.Close()

	bzr := bzip2.NewReader(file)
	return m.extractFromTar(tar.NewReader(bzr), destDir, toolName, binaryPath)
}

// extractTar extracts a plain tar archive.
func (m *ToolManager) extractTar(archive, destDir, toolName, binaryPath string) error {
	//gosec:disable G304 -- archive is temp file path
	file, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("%w: opening archive: %w", ErrToolExtract, err)
	}
	defer file.Close()

	return m.extractFromTar(tar.NewReader(file), destDir, toolName, binaryPath)
}

// extractFromTar extracts a specific file from a tar reader.
func (m *ToolManager) extractFromTar(tr *tar.Reader, destDir, toolName, binaryPath string) error {
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%w: reading tar: %w", ErrToolExtract, err)
		}

		// Clean the path and check for match
		cleanName := filepath.Clean(header.Name)
		if cleanName == binaryPath || strings.HasSuffix(cleanName, "/"+binaryPath) {
			destPath := filepath.Join(destDir, toolName)

			// Security check: ensure dest is within destDir
			if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(destDir)) {
				return fmt.Errorf("%w: path traversal attempt", ErrToolExtract)
			}

			//gosec:disable G304 -- destPath is sanitized
			outFile, err := os.Create(destPath)
			if err != nil {
				return fmt.Errorf("%w: creating output file: %w", ErrToolExtract, err)
			}

			// Limit extraction size to prevent zip bombs (1GB max)
			limited := io.LimitReader(tr, 1<<30)
			_, err = io.Copy(outFile, limited)
			outFile.Close()
			if err != nil {
				return fmt.Errorf("%w: extracting file: %w", ErrToolExtract, err)
			}

			log.Debug().Msgf("Extracted %s from archive to %s", binaryPath, destPath)
			return nil
		}
	}

	return fmt.Errorf("%w: %s", ErrToolBinaryNotFound, binaryPath)
}

// extractZip extracts a specific file from a zip archive.
func (m *ToolManager) extractZip(archive, destDir, toolName, binaryPath string) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("%w: opening zip: %w", ErrToolExtract, err)
	}
	defer r.Close()

	for _, f := range r.File {
		cleanName := filepath.Clean(f.Name)
		if cleanName == binaryPath || strings.HasSuffix(cleanName, "/"+binaryPath) {
			destPath := filepath.Join(destDir, toolName)

			// Security check
			if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(destDir)) {
				return fmt.Errorf("%w: path traversal attempt", ErrToolExtract)
			}

			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("%w: opening file in zip: %w", ErrToolExtract, err)
			}

			//gosec:disable G304 -- destPath is sanitized
			outFile, err := os.Create(destPath)
			if err != nil {
				rc.Close()
				return fmt.Errorf("%w: creating output file: %w", ErrToolExtract, err)
			}

			// Limit extraction size
			limited := io.LimitReader(rc, 1<<30)
			_, err = io.Copy(outFile, limited)
			outFile.Close()
			rc.Close()
			if err != nil {
				return fmt.Errorf("%w: extracting file: %w", ErrToolExtract, err)
			}

			log.Debug().Msgf("Extracted %s from zip to %s", binaryPath, destPath)
			return nil
		}
	}

	return fmt.Errorf("%w: %s", ErrToolBinaryNotFound, binaryPath)
}

// loadMeta loads tool metadata from disk.
func (m *ToolManager) loadMeta(name string) (*ToolMeta, error) {
	metaPath := filepath.Join(m.toolsDir, name+toolMetaSuffix)

	//gosec:disable G304 -- path constructed from state dir + tool name
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}

	var meta ToolMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

// saveMeta saves tool metadata to disk.
func (m *ToolManager) saveMeta(decl ToolDeclaration) error {
	meta := ToolMeta{
		URL:     decl.URL,
		Path:    decl.Path,
		SHA256:  decl.SHA256,
		Retries: decl.Retries,
	}

	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshaling tool meta: %w", err)
	}

	metaPath := filepath.Join(m.toolsDir, decl.Name+toolMetaSuffix)
	if err := os.WriteFile(metaPath, data, fileMode); err != nil {
		return fmt.Errorf("writing tool meta: %w", err)
	}

	return nil
}

// isArchiveURL checks if URL points to an archive file.
func isArchiveURL(url string) bool {
	lower := strings.ToLower(url)
	return strings.HasSuffix(lower, ".tar.gz") ||
		strings.HasSuffix(lower, ".tgz") ||
		strings.HasSuffix(lower, ".tar.bz2") ||
		strings.HasSuffix(lower, ".tar") ||
		strings.HasSuffix(lower, ".zip")
}
