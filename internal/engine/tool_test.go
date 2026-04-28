package engine

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFlow_Tools_PathOnly(t *testing.T) {
	yaml := `
runs-on: host
tools:
  - name: bash
steps:
  - kind: shell
    cmd: echo "hello"
`
	flow, err := ParseFlow([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(flow.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(flow.Tools))
	}

	if flow.Tools[0].Name != "bash" {
		t.Errorf("expected name 'bash', got %q", flow.Tools[0].Name)
	}

	if flow.Tools[0].URL != "" {
		t.Errorf("expected empty URL, got %q", flow.Tools[0].URL)
	}

	if flow.Tools[0].Retries != defaultRetries {
		t.Errorf("expected retries %d, got %d", defaultRetries, flow.Tools[0].Retries)
	}
}

func TestParseFlow_Tools_WithURL(t *testing.T) {
	yaml := `
runs-on: host
tools:
  - name: mytool
    url: https://example.com/mytool
    sha256: abc123
steps:
  - kind: shell
    cmd: mytool --version
`
	flow, err := ParseFlow([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(flow.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(flow.Tools))
	}

	tool := flow.Tools[0]
	if tool.Name != "mytool" {
		t.Errorf("expected name 'mytool', got %q", tool.Name)
	}
	if tool.URL != "https://example.com/mytool" {
		t.Errorf("expected URL 'https://example.com/mytool', got %q", tool.URL)
	}
	if tool.SHA256 != "abc123" {
		t.Errorf("expected sha256 'abc123', got %q", tool.SHA256)
	}
}

func TestParseFlow_Tools_Archive(t *testing.T) {
	yaml := `
runs-on: host
tools:
  - name: kubectl
    url: https://example.com/kubectl.tar.gz
    path: bin/kubectl
    sha256: def456
    retries: 3
steps:
  - kind: shell
    cmd: kubectl version
`
	flow, err := ParseFlow([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(flow.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(flow.Tools))
	}

	tool := flow.Tools[0]
	if tool.Path != "bin/kubectl" {
		t.Errorf("expected path 'bin/kubectl', got %q", tool.Path)
	}
	if tool.Retries != 3 {
		t.Errorf("expected retries 3, got %d", tool.Retries)
	}
}

func TestParseFlow_Tools_MultipleTools(t *testing.T) {
	yaml := `
runs-on: host
tools:
  - name: jq
  - name: yq
    url: https://example.com/yq
steps:
  - kind: shell
    cmd: jq --version
`
	flow, err := ParseFlow([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(flow.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(flow.Tools))
	}

	if flow.Tools[0].Name != "jq" {
		t.Errorf("expected first tool 'jq', got %q", flow.Tools[0].Name)
	}
	if flow.Tools[1].Name != "yq" {
		t.Errorf("expected second tool 'yq', got %q", flow.Tools[1].Name)
	}
}

func TestParseFlow_Tools_MissingName(t *testing.T) {
	yaml := `
runs-on: host
tools:
  - url: https://example.com/tool
steps:
  - kind: shell
    cmd: echo "hello"
`
	_, err := ParseFlow([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing tool name")
	}
}

func TestParseFlow_Tools_DuplicateName(t *testing.T) {
	yaml := `
runs-on: host
tools:
  - name: mytool
  - name: mytool
steps:
  - kind: shell
    cmd: echo "hello"
`
	_, err := ParseFlow([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for duplicate tool name")
	}
}

func TestParseFlow_Tools_InvalidRetries(t *testing.T) {
	yaml := `
runs-on: host
tools:
  - name: mytool
    url: https://example.com/tool
    retries: 0
steps:
  - kind: shell
    cmd: echo "hello"
`
	_, err := ParseFlow([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid retries")
	}
}

func TestParseFlow_Tools_ArchiveWithoutPath(t *testing.T) {
	yaml := `
runs-on: host
tools:
  - name: mytool
    url: https://example.com/tool.tar.gz
steps:
  - kind: shell
    cmd: echo "hello"
`
	_, err := ParseFlow([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for archive without path")
	}
}

func TestParseFlow_Tools_NonArchiveWithPath(t *testing.T) {
	yaml := `
runs-on: host
tools:
  - name: mytool
    url: https://example.com/tool
    path: bin/tool
steps:
  - kind: shell
    cmd: echo "hello"
`
	_, err := ParseFlow([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for non-archive with path")
	}
}

func TestToolManager_Initialize(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "_tools")

	tm := NewToolManager(toolsDir)
	if err := tm.Initialize(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(toolsDir); os.IsNotExist(err) {
		t.Fatal("tools directory was not created")
	}

	if tm.ToolsDir() != toolsDir {
		t.Errorf("expected toolsDir %q, got %q", toolsDir, tm.ToolsDir())
	}
}

func TestToolManager_ResolvePathTool(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewToolManager(filepath.Join(tmpDir, "_tools"))
	if err := tm.Initialize(); err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}

	// bash should exist on most systems
	err := tm.Resolve([]ToolDeclaration{{Name: "bash"}})
	if err != nil {
		t.Fatalf("unexpected error resolving bash: %v", err)
	}
}

func TestToolManager_ResolvePathTool_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewToolManager(filepath.Join(tmpDir, "_tools"))
	if err := tm.Initialize(); err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}

	err := tm.Resolve([]ToolDeclaration{{Name: "nonexistent-tool-xyz-123"}})
	if err == nil {
		t.Fatal("expected error for nonexistent tool")
	}
}

func TestToolManager_DownloadBinary(t *testing.T) {
	// Create test server
	binaryContent := "#!/bin/bash\necho 'hello'"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(binaryContent))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	tm := NewToolManager(filepath.Join(tmpDir, "_tools"))
	if err := tm.Initialize(); err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}

	err := tm.Resolve([]ToolDeclaration{
		{Name: "mytool", URL: server.URL + "/mytool", Retries: 1},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify binary exists and is executable
	toolPath := filepath.Join(tm.ToolsDir(), "mytool")
	info, err := os.Stat(toolPath)
	if err != nil {
		t.Fatalf("tool not found: %v", err)
	}

	if info.Mode()&0111 == 0 {
		t.Error("tool is not executable")
	}

	// Verify meta file exists
	metaPath := filepath.Join(tm.ToolsDir(), "mytool.meta.json")
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		t.Error("meta file not created")
	}
}

func TestToolManager_DownloadBinary_WithChecksum(t *testing.T) {
	binaryContent := "#!/bin/bash\necho 'hello'"
	h := sha256.Sum256([]byte(binaryContent))
	checksum := hex.EncodeToString(h[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(binaryContent))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	tm := NewToolManager(filepath.Join(tmpDir, "_tools"))
	if err := tm.Initialize(); err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}

	err := tm.Resolve([]ToolDeclaration{
		{Name: "mytool", URL: server.URL + "/mytool", SHA256: checksum, Retries: 1},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestToolManager_DownloadBinary_ChecksumMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("binary content"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	tm := NewToolManager(filepath.Join(tmpDir, "_tools"))
	if err := tm.Initialize(); err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}

	err := tm.Resolve([]ToolDeclaration{
		{Name: "mytool", URL: server.URL + "/mytool", SHA256: "wrong-checksum", Retries: 1},
	})
	if err == nil {
		t.Fatal("expected error for checksum mismatch")
	}
}

func TestToolManager_DownloadBinary_Cached(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write([]byte("binary content"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	tm := NewToolManager(filepath.Join(tmpDir, "_tools"))
	if err := tm.Initialize(); err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}

	decl := ToolDeclaration{Name: "mytool", URL: server.URL + "/mytool", Retries: 1}

	// First resolve
	if err := tm.Resolve([]ToolDeclaration{decl}); err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}

	// Second resolve should use cache
	if err := tm.Resolve([]ToolDeclaration{decl}); err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 download, got %d", callCount)
	}
}

func TestToolManager_DownloadBinary_CacheInvalidation(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write([]byte("binary content"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	tm := NewToolManager(filepath.Join(tmpDir, "_tools"))
	if err := tm.Initialize(); err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}

	// First resolve
	decl1 := ToolDeclaration{Name: "mytool", URL: server.URL + "/v1/mytool", Retries: 1}
	if err := tm.Resolve([]ToolDeclaration{decl1}); err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}

	// Second resolve with different URL should redownload
	decl2 := ToolDeclaration{Name: "mytool", URL: server.URL + "/v2/mytool", Retries: 1}
	if err := tm.Resolve([]ToolDeclaration{decl2}); err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 downloads due to cache invalidation, got %d", callCount)
	}
}

func TestToolManager_DownloadWithRetries(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte("binary content"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	tm := NewToolManager(filepath.Join(tmpDir, "_tools"))
	if err := tm.Initialize(); err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}

	err := tm.Resolve([]ToolDeclaration{
		{Name: "mytool", URL: server.URL + "/mytool", Retries: 3},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if callCount != 3 {
		t.Errorf("expected 3 attempts, got %d", callCount)
	}
}

func TestToolManager_ExtractTarGz(t *testing.T) {
	// Create a tar.gz archive in memory
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")

	// Create archive
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("failed to create archive file: %v", err)
	}

	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	binaryContent := "#!/bin/bash\necho 'extracted'"
	hdr := &tar.Header{
		Name: "bin/mytool",
		Mode: 0755,
		Size: int64(len(binaryContent)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("failed to write tar header: %v", err)
	}
	if _, err := tw.Write([]byte(binaryContent)); err != nil {
		t.Fatalf("failed to write tar content: %v", err)
	}
	tw.Close()
	gzw.Close()
	f.Close()

	// Create checksum
	archiveData, _ := os.ReadFile(archivePath)
	h := sha256.Sum256(archiveData)
	checksum := hex.EncodeToString(h[:])

	// Serve the archive
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, archivePath)
	}))
	defer server.Close()

	toolsDir := filepath.Join(tmpDir, "_tools")
	tm := NewToolManager(toolsDir)
	if err := tm.Initialize(); err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}

	err = tm.Resolve([]ToolDeclaration{
		{
			Name:    "mytool",
			URL:     server.URL + "/test.tar.gz",
			Path:    "bin/mytool",
			SHA256:  checksum,
			Retries: 1,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify extracted binary
	toolPath := filepath.Join(toolsDir, "mytool")
	content, err := os.ReadFile(toolPath)
	if err != nil {
		t.Fatalf("failed to read extracted tool: %v", err)
	}
	if string(content) != binaryContent {
		t.Errorf("expected content %q, got %q", binaryContent, string(content))
	}

	// Verify executable
	info, err := os.Stat(toolPath)
	if err != nil {
		t.Fatalf("failed to stat tool: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Error("extracted tool is not executable")
	}
}

func TestToolManager_ExtractZip(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.zip")

	// Create zip archive
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("failed to create archive file: %v", err)
	}

	zw := zip.NewWriter(f)
	binaryContent := "#!/bin/bash\necho 'from zip'"

	w, err := zw.Create("bin/mytool")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	if _, err := w.Write([]byte(binaryContent)); err != nil {
		t.Fatalf("failed to write zip content: %v", err)
	}
	zw.Close()
	f.Close()

	// Create checksum
	archiveData, _ := os.ReadFile(archivePath)
	h := sha256.Sum256(archiveData)
	checksum := hex.EncodeToString(h[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, archivePath)
	}))
	defer server.Close()

	toolsDir := filepath.Join(tmpDir, "_tools")
	tm := NewToolManager(toolsDir)
	if err := tm.Initialize(); err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}

	err = tm.Resolve([]ToolDeclaration{
		{
			Name:    "mytool",
			URL:     server.URL + "/test.zip",
			Path:    "bin/mytool",
			SHA256:  checksum,
			Retries: 1,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify extracted binary
	toolPath := filepath.Join(toolsDir, "mytool")
	content, err := os.ReadFile(toolPath)
	if err != nil {
		t.Fatalf("failed to read extracted tool: %v", err)
	}
	if string(content) != binaryContent {
		t.Errorf("expected content %q, got %q", binaryContent, string(content))
	}
}

func TestToolManager_ExtractArchive_BinaryNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")

	// Create empty archive
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("failed to create archive file: %v", err)
	}

	gzw := gzip.NewWriter(f)
	tw := tar.NewWriter(gzw)

	// Add a different file
	hdr := &tar.Header{
		Name: "other/file",
		Mode: 0644,
		Size: 5,
	}
	tw.WriteHeader(hdr)
	tw.Write([]byte("hello"))
	tw.Close()
	gzw.Close()
	f.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, archivePath)
	}))
	defer server.Close()

	toolsDir := filepath.Join(tmpDir, "_tools")
	tm := NewToolManager(toolsDir)
	if err := tm.Initialize(); err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}

	err = tm.Resolve([]ToolDeclaration{
		{
			Name:    "mytool",
			URL:     server.URL + "/test.tar.gz",
			Path:    "bin/mytool",
			Retries: 1,
		},
	})
	if err == nil {
		t.Fatal("expected error for missing binary in archive")
	}
}

func TestIsArchiveURL(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"https://example.com/tool.tar.gz", true},
		{"https://example.com/tool.tgz", true},
		{"https://example.com/tool.tar.bz2", true},
		{"https://example.com/tool.tar", true},
		{"https://example.com/tool.zip", true},
		{"https://example.com/tool.TAR.GZ", true},
		{"https://example.com/tool", false},
		{"https://example.com/tool.exe", false},
		{"https://example.com/tool.bin", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := isArchiveURL(tt.url)
			if result != tt.expected {
				t.Errorf("isArchiveURL(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}

func TestEngine_Run_WithTools_PATH(t *testing.T) {
	yaml := `
runs-on: host
tools:
  - name: bash
steps:
  - kind: shell
    cmd: echo "tool resolved"
`
	engine := NewEngine()
	flow, err := engine.LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("failed to load flow: %v", err)
	}

	var output []byte
	err = engine.Run(context.Background(), flow, WithLogWriter(&testWriter{data: &output}))
	if err != nil {
		t.Fatalf("failed to run flow: %v", err)
	}
}

func TestEngine_Run_WithTools_Download(t *testing.T) {
	binaryContent := "#!/usr/bin/env bash\necho 'downloaded tool'"
	h := sha256.Sum256([]byte(binaryContent))
	checksum := hex.EncodeToString(h[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(binaryContent))
	}))
	defer server.Close()

	yaml := fmt.Sprintf(`
runs-on: host
tools:
  - name: mytool
    url: %s/mytool
    sha256: %s
steps:
  - kind: shell
    cmd: mytool
`, server.URL, checksum)

	engine := NewEngine()
	flow, err := engine.LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("failed to load flow: %v", err)
	}

	var output []byte
	err = engine.Run(context.Background(), flow, WithLogWriter(&testWriter{data: &output}))
	if err != nil {
		t.Fatalf("failed to run flow: %v", err)
	}

	if !strings.Contains(string(output), "downloaded tool") {
		t.Errorf("expected output to contain 'downloaded tool', got %q", string(output))
	}
}

func TestEngine_Run_WithTools_NotFound(t *testing.T) {
	yaml := `
runs-on: host
tools:
  - name: nonexistent-tool-xyz-123
steps:
  - kind: shell
    cmd: echo "hello"
`
	engine := NewEngine()
	flow, err := engine.LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("failed to load flow: %v", err)
	}

	err = engine.Run(context.Background(), flow)
	if err == nil {
		t.Fatal("expected error for missing tool")
	}
}

// testWriter is a simple io.Writer for capturing output
type testWriter struct {
	data *[]byte
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	*w.data = append(*w.data, p...)
	return len(p), nil
}
