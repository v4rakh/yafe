package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStateManager_DefaultDir(t *testing.T) {
	sm := NewStateManager("", "test-run-123")

	assert.Contains(t, sm.BaseDir(), "yafe-test-run-123")
	assert.Equal(t, "test-run-123", sm.RunID())
}

func TestNewStateManager_CustomDir(t *testing.T) {
	sm := NewStateManager("/custom/path", "test-run-123")

	assert.Equal(t, "/custom/path", sm.BaseDir())
	assert.Equal(t, "test-run-123", sm.RunID())
}

func TestStateManager_Initialize(t *testing.T) {
	tempDir := t.TempDir()
	sm := NewStateManager(tempDir, "test-run")

	err := sm.Initialize()

	require.NoError(t, err)
	assert.DirExists(t, tempDir)
	assert.DirExists(t, filepath.Join(tempDir, outputsDirName))
}

func TestStateManager_StepDir(t *testing.T) {
	sm := NewStateManager("/tmp/yafe-test", "test-run")

	stepDir := sm.StepDir("build")

	assert.Equal(t, "/tmp/yafe-test/build", stepDir)
}

func TestStateManager_OutputFile(t *testing.T) {
	sm := NewStateManager("/tmp/yafe-test", "test-run")

	outputFile := sm.OutputFile("build")

	assert.Equal(t, "/tmp/yafe-test/"+outputsDirName+"/build", outputFile)
}

func TestStateManager_PrepareStep(t *testing.T) {
	tempDir := t.TempDir()
	sm := NewStateManager(tempDir, "test-run")
	require.NoError(t, sm.Initialize())

	err := sm.PrepareStep("build")

	require.NoError(t, err)
	assert.DirExists(t, filepath.Join(tempDir, "build"))
	assert.FileExists(t, filepath.Join(tempDir, outputsDirName, "build"))
}

func TestStateManager_CollectOutputs_Variable(t *testing.T) {
	tempDir := t.TempDir()
	sm := NewStateManager(tempDir, "test-run")
	require.NoError(t, sm.Initialize())
	require.NoError(t, sm.PrepareStep("build"))

	outputFile := sm.OutputFile("build")
	err := os.WriteFile(outputFile, []byte("version=1.0.0\n"), fileMode)
	require.NoError(t, err)

	step := &ShellStep{
		Name: "build",
		Outputs: []StepOutput{
			{Name: "version", Type: OutputTypeVariable},
		},
	}

	err = sm.CollectOutputs(step)

	require.NoError(t, err)
	value, found := sm.GetOutput("build", "version")
	assert.True(t, found)
	assert.Equal(t, "1.0.0", value)
}

func TestStateManager_CollectOutputs_File(t *testing.T) {
	tempDir := t.TempDir()
	sm := NewStateManager(tempDir, "test-run")
	require.NoError(t, sm.Initialize())
	require.NoError(t, sm.PrepareStep("build"))

	outputPath := filepath.Join(tempDir, "build", "artifact.zip")
	require.NoError(t, os.WriteFile(outputPath, []byte("fake zip content"), fileMode))

	step := &ShellStep{
		Name: "build",
		Outputs: []StepOutput{
			{Name: "artifact", Type: OutputTypeFile, Path: "build/artifact.zip"},
		},
	}

	err := sm.CollectOutputs(step)

	require.NoError(t, err)
	value, found := sm.GetOutput("build", "artifact")
	assert.True(t, found)
	assert.Equal(t, filepath.Join(tempDir, "build", "artifact.zip"), value)
}

func TestStateManager_CollectOutputs_MissingVariable(t *testing.T) {
	tempDir := t.TempDir()
	sm := NewStateManager(tempDir, "test-run")
	require.NoError(t, sm.Initialize())
	require.NoError(t, sm.PrepareStep("build"))

	step := &ShellStep{
		Name: "build",
		Outputs: []StepOutput{
			{Name: "version", Type: OutputTypeVariable},
		},
	}

	err := sm.CollectOutputs(step)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not produce variable output")
}

func TestStateManager_CollectOutputs_MissingFile(t *testing.T) {
	tempDir := t.TempDir()
	sm := NewStateManager(tempDir, "test-run")
	require.NoError(t, sm.Initialize())
	require.NoError(t, sm.PrepareStep("build"))

	step := &ShellStep{
		Name: "build",
		Outputs: []StepOutput{
			{Name: "artifact", Type: OutputTypeFile, Path: "build/artifact.zip"},
		},
	}

	err := sm.CollectOutputs(step)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not produce file")
}

func TestStateManager_CollectOutputs_MultipleVariables(t *testing.T) {
	tempDir := t.TempDir()
	sm := NewStateManager(tempDir, "test-run")
	require.NoError(t, sm.Initialize())
	require.NoError(t, sm.PrepareStep("build"))

	outputFile := sm.OutputFile("build")
	err := os.WriteFile(outputFile, []byte("version=1.0.0\ncommit=abc123\nbranch=main\n"), fileMode)
	require.NoError(t, err)

	step := &ShellStep{
		Name: "build",
		Outputs: []StepOutput{
			{Name: "version", Type: OutputTypeVariable},
			{Name: "commit", Type: OutputTypeVariable},
			{Name: "branch", Type: OutputTypeVariable},
		},
	}

	err = sm.CollectOutputs(step)

	require.NoError(t, err)

	version, _ := sm.GetOutput("build", "version")
	assert.Equal(t, "1.0.0", version)

	commit, _ := sm.GetOutput("build", "commit")
	assert.Equal(t, "abc123", commit)

	branch, _ := sm.GetOutput("build", "branch")
	assert.Equal(t, "main", branch)
}

func TestStateManager_GetOutput_NotFound(t *testing.T) {
	sm := NewStateManager("", "test-run")

	_, found := sm.GetOutput("build", "version")

	assert.False(t, found)
}

func TestStateManager_Cleanup(t *testing.T) {
	tempDir := t.TempDir()
	testDir := filepath.Join(tempDir, "yafe-test")
	sm := NewStateManager(testDir, "test-run")
	require.NoError(t, sm.Initialize())
	require.NoError(t, sm.PrepareStep("build"))

	err := sm.Cleanup()

	require.NoError(t, err)
	_, err = os.Stat(testDir)
	assert.True(t, os.IsNotExist(err))
}

func TestStateManager_CollectOutputs_PathTraversal(t *testing.T) {
	tempDir := t.TempDir()
	sm := NewStateManager(tempDir, "test-run")
	require.NoError(t, sm.Initialize())
	require.NoError(t, sm.PrepareStep("build"))

	step := &ShellStep{
		Name: "build",
		Outputs: []StepOutput{
			{Name: "evil", Type: OutputTypeFile, Path: "../../../etc/passwd"},
		},
	}

	err := sm.CollectOutputs(step)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPathTraversal)
	assert.Contains(t, err.Error(), "escapes state directory")
}
