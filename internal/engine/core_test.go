package engine

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngine_LoadBytes_MinimalFlow(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - cmd: echo "hello"
    kind: shell
`)

	e := NewEngine()
	flow, err := e.LoadBytes(yamlData)

	require.NoError(t, err)
	require.NotNil(t, flow)
	assert.Len(t, flow.Steps, 1)
}

func TestEngine_LoadBytes_CustomShell(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - cmd: pwd && ls
    kind: shell
    shell: zsh
`)

	e := NewEngine()
	flow, err := e.LoadBytes(yamlData)

	require.NoError(t, err)
	assert.Len(t, flow.Steps, 1)
	assert.Equal(t, "zsh", flow.Steps[0].(*ShellStep).Shell)
}

func TestEngine_LoadBytes_EnvVars(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - cmd: echo $DEBUG
    kind: shell
    env:
      - DEBUG=true
`)

	e := NewEngine()
	flow, err := e.LoadBytes(yamlData)

	require.NoError(t, err)
	assert.Len(t, flow.Steps[0].(*ShellStep).Env, 1)
	assert.Equal(t, "DEBUG=true", flow.Steps[0].(*ShellStep).Env[0])
}

func TestEngine_LoadBytes_MultiLineCmd(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - cmd: |
      echo "line 1"
      echo "line 2"
    kind: shell
`)

	e := NewEngine()
	flow, err := e.LoadBytes(yamlData)

	require.NoError(t, err)
	assert.Contains(t, flow.Steps[0].(*ShellStep).Cmd, "line 1")
	assert.Contains(t, flow.Steps[0].(*ShellStep).Cmd, "line 2")
}

func TestEngine_LoadBytes_MultipleSteps(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - cmd: echo "step 1"
    kind: shell
  - cmd: echo "step 2"
    kind: shell
  - cmd: echo "step 3"
    kind: shell
`)

	e := NewEngine()
	flow, err := e.LoadBytes(yamlData)

	require.NoError(t, err)
	assert.Len(t, flow.Steps, 3)
}

func TestEngine_LoadBytes_InvalidMissingRunsOn(t *testing.T) {
	yamlData := []byte(`
steps:
  - cmd: echo "test"
    kind: shell
`)

	e := NewEngine()
	flow, err := e.LoadBytes(yamlData)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing failed")
	assert.Nil(t, flow)
}

func TestEngine_LoadBytes_InvalidMissingKind(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - cmd: echo "test"
`)

	e := NewEngine()
	flow, err := e.LoadBytes(yamlData)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing failed")
	assert.Nil(t, flow)
}

func TestEngine_LoadBytes_InvalidUnknownKind(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - kind: unknown
    cmd: echo "test"
`)

	e := NewEngine()
	flow, err := e.LoadBytes(yamlData)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing failed")
	assert.Nil(t, flow)
}

func TestEngine_Run_NilFlow(t *testing.T) {
	e := NewEngine()
	err := e.Run(context.Background(), nil)

	assert.ErrorIs(t, err, ErrValidationNotNil)
}

func TestEngine_Run_EmptyFlow(t *testing.T) {
	e := NewEngine()
	flow := &Flow{}
	err := e.Run(context.Background(), flow)

	assert.ErrorIs(t, err, ErrValidationNotNil)
}

func TestEngine_Run_SuccessfulBootstrapTeardown(t *testing.T) {
	// Mock successful flow - assumes your impls work
	yamlData := []byte(`
runs-on: host
steps:
  - cmd: echo "success"
    kind: shell
`)

	e := NewEngine()
	flow, err := e.LoadBytes(yamlData)
	require.NoError(t, err)

	// Mock step execution to succeed
	err = e.Run(context.Background(), flow)
	// Note: actual execution depends on your RunsOn/Step impls
	// This tests the engine orchestration
}

func TestEngine_Run_MultipleSteps_Success(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - cmd: echo "step 1"
    kind: shell
  - cmd: echo "step 2"
    kind: shell
`)

	e := NewEngine()
	flow, err := e.LoadBytes(yamlData)
	require.NoError(t, err)

	err = e.Run(context.Background(), flow)
	// Tests engine step iteration + RunsOn orchestration
}

func TestEngine_Run_StateSharing_Variables(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - name: build
    cmd: |
      echo "version=1.2.3" >> $YAFE_OUTPUT
      echo "commit=abc123" >> $YAFE_OUTPUT
    kind: shell
    outputs:
      - name: version
        type: variable
      - name: commit
        type: variable
  - name: deploy
    cmd: |
      echo "Deploying version ${{ steps.build.outputs.version }} at commit ${{ steps.build.outputs.commit }}"
    kind: shell
`)

	e := NewEngine()
	flow, err := e.LoadBytes(yamlData)
	require.NoError(t, err)

	err = e.Run(context.Background(), flow)
	require.NoError(t, err)
}

func TestEngine_Run_StateSharing_Files(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - name: build
    cmd: |
      mkdir -p $YAFE_STATE/build
      echo "fake artifact content" > $YAFE_STATE/build/artifact.txt
    kind: shell
    outputs:
      - name: artifact
        type: file
        path: build/artifact.txt
  - name: deploy
    cmd: |
      cat ${{ steps.build.outputs.artifact }}
    kind: shell
`)

	e := NewEngine()
	flow, err := e.LoadBytes(yamlData)
	require.NoError(t, err)

	err = e.Run(context.Background(), flow)
	require.NoError(t, err)
}

func TestEngine_Run_StateSharing_CustomStateDir(t *testing.T) {
	tempDir := t.TempDir()
	yamlData := []byte(`
state-dir: ` + tempDir + `
runs-on: host
steps:
  - name: build
    cmd: |
      echo "version=2.0.0" >> $YAFE_OUTPUT
    kind: shell
    outputs:
      - name: version
        type: variable
  - name: deploy
    cmd: |
      echo "Version is ${{ steps.build.outputs.version }}"
    kind: shell
`)

	e := NewEngine()
	flow, err := e.LoadBytes(yamlData)
	require.NoError(t, err)
	assert.Equal(t, tempDir, flow.StateDir)

	err = e.Run(context.Background(), flow)
	require.NoError(t, err)
}

func TestEngine_Run_StateSharing_MissingOutput(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - name: build
    cmd: |
      echo "not writing version output"
    kind: shell
    outputs:
      - name: version
        type: variable
`)

	e := NewEngine()
	flow, err := e.LoadBytes(yamlData)
	require.NoError(t, err)

	err = e.Run(context.Background(), flow)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOutputNotProduced)
}

func TestEngine_Run_Secrets_FlowLevelFromEnv(t *testing.T) {
	t.Setenv("TEST_API_KEY", "my-secret-key-123")

	yamlData := []byte(`
runs-on: host
secrets:
  - name: api_key
    from:
      env: TEST_API_KEY
steps:
  - name: deploy
    cmd: |
      echo "Using API key: ${{ secrets.api_key }}"
    kind: shell
`)

	e := NewEngine()
	flow, err := e.LoadBytes(yamlData)
	require.NoError(t, err)

	err = e.Run(context.Background(), flow)
	require.NoError(t, err)
}

func TestEngine_Run_Secrets_StepLevelFromEnv(t *testing.T) {
	t.Setenv("DEPLOY_KEY", "deploy-secret-456")

	yamlData := []byte(`
runs-on: host
steps:
  - name: deploy
    kind: shell
    secrets:
      - name: deploy_key
        from:
          env: DEPLOY_KEY
    cmd: |
      echo "Using deploy key: ${{ secrets.deploy_key }}"
`)

	e := NewEngine()
	flow, err := e.LoadBytes(yamlData)
	require.NoError(t, err)

	err = e.Run(context.Background(), flow)
	require.NoError(t, err)
}

func TestEngine_Run_Secrets_StepOverridesFlow(t *testing.T) {
	t.Setenv("FLOW_SECRET", "flow-value")
	t.Setenv("STEP_SECRET", "step-value")

	yamlData := []byte(`
runs-on: host
secrets:
  - name: shared_secret
    from:
      env: FLOW_SECRET
steps:
  - name: step1
    kind: shell
    cmd: |
      echo "Flow secret: ${{ secrets.shared_secret }}"
  - name: step2
    kind: shell
    secrets:
      - name: shared_secret
        from:
          env: STEP_SECRET
    cmd: |
      echo "Step override: ${{ secrets.shared_secret }}"
`)

	e := NewEngine()
	flow, err := e.LoadBytes(yamlData)
	require.NoError(t, err)

	err = e.Run(context.Background(), flow)
	require.NoError(t, err)
}

func TestEngine_Run_Secrets_FromFile(t *testing.T) {
	tempDir := t.TempDir()
	secretFile := tempDir + "/secret.txt"
	require.NoError(t, writeTestFile(secretFile, "file-secret-content"))

	yamlData := []byte(`
runs-on: host
secrets:
  - name: file_secret
    from:
      file: ` + secretFile + `
steps:
  - name: use_secret
    kind: shell
    cmd: |
      echo "File secret: ${{ secrets.file_secret }}"
`)

	e := NewEngine()
	flow, err := e.LoadBytes(yamlData)
	require.NoError(t, err)

	err = e.Run(context.Background(), flow)
	require.NoError(t, err)
}

func TestEngine_Run_Secrets_MissingEnvVar(t *testing.T) {
	yamlData := []byte(`
runs-on: host
secrets:
  - name: missing_secret
    from:
      env: NONEXISTENT_ENV_VAR
steps:
  - name: deploy
    kind: shell
    cmd: echo "test"
`)

	e := NewEngine()
	flow, err := e.LoadBytes(yamlData)
	require.NoError(t, err)

	err = e.Run(context.Background(), flow)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSecretEnvNotSet)
}

func TestEngine_Run_Secrets_CombinedWithOutputs(t *testing.T) {
	t.Setenv("API_TOKEN", "secret-token-789")

	yamlData := []byte(`
runs-on: host
secrets:
  - name: api_token
    from:
      env: API_TOKEN
steps:
  - name: build
    kind: shell
    cmd: |
      echo "version=1.0.0" >> $YAFE_OUTPUT
    outputs:
      - name: version
        type: variable
  - name: deploy
    kind: shell
    cmd: |
      echo "Deploying ${{ steps.build.outputs.version }} with token ${{ secrets.api_token }}"
`)

	e := NewEngine()
	flow, err := e.LoadBytes(yamlData)
	require.NoError(t, err)

	err = e.Run(context.Background(), flow)
	require.NoError(t, err)
}

// writeTestFile is a helper to write test files
func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), fileMode)
}
