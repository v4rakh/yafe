package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFlow_MinimalValid(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - cmd: echo "hello"
    kind: shell
`)

	flow, err := ParseFlow(yamlData)

	require.NoError(t, err)
	require.NotNil(t, flow)
	assert.IsType(t, &RunsOnHost{}, flow.RunsOn)
	assert.Len(t, flow.Steps, 1)
	assert.Equal(t, StepKindShell, flow.Steps[0].Kind())
}

func TestParseFlow_CustomShell(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - cmd: pwd
    kind: shell
    shell: zsh
`)

	flow, err := ParseFlow(yamlData)

	require.NoError(t, err)
	shellStep := flow.Steps[0].(*ShellStep)
	assert.Equal(t, "zsh", shellStep.Shell)
	assert.Equal(t, "pwd", shellStep.Cmd)
}

func TestParseFlow_EnvVars(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - cmd: echo $DEBUG
    kind: shell
    env:
      - DEBUG=true
      - NODE_ENV=prod
`)

	flow, err := ParseFlow(yamlData)

	require.NoError(t, err)
	shellStep := flow.Steps[0].(*ShellStep)
	assert.Equal(t, []string{"DEBUG=true", "NODE_ENV=prod"}, shellStep.Env)
}

func TestParseFlow_MultiLineCmd(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - cmd: |
      echo "line 1"
      echo "line 2"
    kind: shell
`)

	flow, err := ParseFlow(yamlData)

	require.NoError(t, err)
	shellStep := flow.Steps[0].(*ShellStep)
	assert.Contains(t, shellStep.Cmd, "line 1")
	assert.Contains(t, shellStep.Cmd, "line 2")
}

func TestParseFlow_MultipleSteps(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - cmd: echo "step 1"
    kind: shell
  - cmd: echo "step 2"
    kind: shell
`)

	flow, err := ParseFlow(yamlData)

	require.NoError(t, err)
	assert.Len(t, flow.Steps, 2)
	assert.Equal(t, StepKindShell, flow.Steps[0].Kind())
	assert.Equal(t, StepKindShell, flow.Steps[1].Kind())
}

func TestParseFlow_MissingRunsOn(t *testing.T) {
	yamlData := []byte(`
steps:
  - cmd: echo "test"
    kind: shell
`)

	flow, err := ParseFlow(yamlData)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "runs-on: must be provided")
	assert.Nil(t, flow)
}

func TestParseFlow_InvalidRunsOn(t *testing.T) {
	yamlData := []byte(`
runs-on: ubuntu
steps:
  - cmd: echo "test"
    kind: shell
`)

	flow, err := ParseFlow(yamlData)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "runs-on: unknown ubuntu")
	assert.Nil(t, flow)
}

func TestParseFlow_MissingSteps(t *testing.T) {
	yamlData := []byte(`
runs-on: host
`)

	flow, err := ParseFlow(yamlData)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "steps: must be provided")
	assert.Nil(t, flow)
}

func TestParseFlow_StepMissingKind(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - cmd: echo "test"
`)

	flow, err := ParseFlow(yamlData)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "step 0: missing kind field")
	assert.Nil(t, flow)
}

func TestParseFlow_StepUnknownKind(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - kind: unknown
    cmd: echo "test"
`)

	flow, err := ParseFlow(yamlData)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "step 0: unknown kind unknown")
	assert.Nil(t, flow)
}

func TestParseFlow_StepNotMap(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - "not a map"
`)

	flow, err := ParseFlow(yamlData)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid yaml contents")
	assert.Nil(t, flow)
}

func TestParseFlow_StepInvalidKindType(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - kind: 123
    cmd: echo "test"
`)

	flow, err := ParseFlow(yamlData)

	require.Error(t, err)
	// YAML unmarshals 123 as string "123", which fails as unknown kind
	assert.Contains(t, err.Error(), "unknown kind 123")
	assert.Nil(t, flow)
}

func TestParseFlow_InvalidYAML(t *testing.T) {
	yamlData := []byte("invalid: yaml: syntax")

	flow, err := ParseFlow(yamlData)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid yaml contents")
	assert.Nil(t, flow)
}

func TestParseFlow_ShellDefaultsApplied(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - cmd: echo "test"
    kind: shell
`)

	flow, err := ParseFlow(yamlData)

	require.NoError(t, err)
	shellStep := flow.Steps[0].(*ShellStep)
	assert.Equal(t, defaultShell, shellStep.Shell)
}

func TestParseFlow_StateDir(t *testing.T) {
	yamlData := []byte(`
state-dir: /custom/state
runs-on: host
steps:
  - cmd: echo "test"
    kind: shell
`)

	flow, err := ParseFlow(yamlData)

	require.NoError(t, err)
	assert.Equal(t, "/custom/state", flow.StateDir)
}

func TestParseFlow_StepName(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - name: build
    cmd: echo "building"
    kind: shell
`)

	flow, err := ParseFlow(yamlData)

	require.NoError(t, err)
	shellStep := flow.Steps[0].(*ShellStep)
	assert.Equal(t, "build", shellStep.Name)
}

func TestParseFlow_StepOutputs(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - name: build
    cmd: echo "version=1.0.0" >> $YAFE_OUTPUT
    kind: shell
    outputs:
      - name: version
        type: variable
      - name: artifact
        type: file
        path: build/artifact.zip
`)

	flow, err := ParseFlow(yamlData)

	require.NoError(t, err)
	shellStep := flow.Steps[0].(*ShellStep)
	require.Len(t, shellStep.Outputs, 2)

	assert.Equal(t, "version", shellStep.Outputs[0].Name)
	assert.Equal(t, OutputTypeVariable, shellStep.Outputs[0].Type)

	assert.Equal(t, "artifact", shellStep.Outputs[1].Name)
	assert.Equal(t, OutputTypeFile, shellStep.Outputs[1].Type)
	assert.Equal(t, "build/artifact.zip", shellStep.Outputs[1].Path)
}

func TestParseFlow_DuplicateStepNames(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - name: build
    cmd: echo "building"
    kind: shell
  - name: build
    cmd: echo "building again"
    kind: shell
`)

	flow, err := ParseFlow(yamlData)

	require.Error(t, err)
	assert.Nil(t, flow)
	assert.Contains(t, err.Error(), "duplicate step name")
}

func TestParseFlow_InvalidOutputType(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - name: build
    cmd: echo "building"
    kind: shell
    outputs:
      - name: version
        type: invalid
`)

	flow, err := ParseFlow(yamlData)

	require.Error(t, err)
	assert.Nil(t, flow)
	assert.ErrorIs(t, err, ErrInvalidOutputType)
}

func TestParseFlow_FileOutputWithoutPath(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - name: build
    cmd: echo "building"
    kind: shell
    outputs:
      - name: artifact
        type: file
`)

	flow, err := ParseFlow(yamlData)

	require.Error(t, err)
	assert.Nil(t, flow)
	assert.Contains(t, err.Error(), "file output requires path")
}

func TestParseFlow_OutputsWithoutName(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - cmd: echo "building"
    kind: shell
    outputs:
      - name: version
        type: variable
`)

	flow, err := ParseFlow(yamlData)

	require.Error(t, err)
	assert.Nil(t, flow)
	assert.Contains(t, err.Error(), "steps with outputs must have a name")
}

func TestParseFlow_TemplateReference(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - name: build
    cmd: echo "version=1.0.0" >> $YAFE_OUTPUT
    kind: shell
    outputs:
      - name: version
        type: variable
  - name: deploy
    cmd: echo "Deploying ${{ steps.build.outputs.version }}"
    kind: shell
`)

	flow, err := ParseFlow(yamlData)

	require.NoError(t, err)
	assert.NotNil(t, flow)
}

func TestParseFlow_TemplateReferenceInvalidStep(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - name: deploy
    cmd: echo "Deploying ${{ steps.build.outputs.version }}"
    kind: shell
`)

	flow, err := ParseFlow(yamlData)

	require.Error(t, err)
	assert.Nil(t, flow)
	assert.Contains(t, err.Error(), "non-existent step")
}

func TestParseFlow_TemplateReferenceWrongOrder(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - name: deploy
    cmd: echo "Deploying ${{ steps.build.outputs.version }}"
    kind: shell
  - name: build
    cmd: echo "version=1.0.0" >> $YAFE_OUTPUT
    kind: shell
    outputs:
      - name: version
        type: variable
`)

	flow, err := ParseFlow(yamlData)

	require.Error(t, err)
	assert.Nil(t, flow)
	assert.Contains(t, err.Error(), "not executed before")
}

func TestParseFlow_FlowSecrets(t *testing.T) {
	yamlData := []byte(`
runs-on: host
secrets:
  - name: api_key
    from:
      env: API_KEY
  - name: db_password
    from:
      file: /run/secrets/db
steps:
  - name: deploy
    cmd: echo "Using ${{ secrets.api_key }}"
    kind: shell
`)

	flow, err := ParseFlow(yamlData)

	require.NoError(t, err)
	require.Len(t, flow.Secrets, 2)

	assert.Equal(t, "api_key", flow.Secrets[0].Name)
	assert.Equal(t, "API_KEY", flow.Secrets[0].From.Env)

	assert.Equal(t, "db_password", flow.Secrets[1].Name)
	assert.Equal(t, "/run/secrets/db", flow.Secrets[1].From.File)
}

func TestParseFlow_StepSecrets(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - name: deploy
    kind: shell
    secrets:
      - name: deploy_key
        from:
          env: DEPLOY_KEY
    cmd: echo "Using ${{ secrets.deploy_key }}"
`)

	flow, err := ParseFlow(yamlData)

	require.NoError(t, err)
	shellStep := flow.Steps[0].(*ShellStep)
	require.Len(t, shellStep.Secrets, 1)
	assert.Equal(t, "deploy_key", shellStep.Secrets[0].Name)
	assert.Equal(t, "DEPLOY_KEY", shellStep.Secrets[0].From.Env)
}

func TestParseFlow_SecretMissingName(t *testing.T) {
	yamlData := []byte(`
runs-on: host
secrets:
  - from:
      env: API_KEY
steps:
  - cmd: echo "test"
    kind: shell
`)

	flow, err := ParseFlow(yamlData)

	require.Error(t, err)
	assert.Nil(t, flow)
	assert.Contains(t, err.Error(), "name is required")
}

func TestParseFlow_SecretMissingSource(t *testing.T) {
	yamlData := []byte(`
runs-on: host
secrets:
  - name: api_key
steps:
  - cmd: echo "test"
    kind: shell
`)

	flow, err := ParseFlow(yamlData)

	require.Error(t, err)
	assert.Nil(t, flow)
	assert.ErrorIs(t, err, ErrSecretSourceEmpty)
}

func TestParseFlow_DuplicateFlowSecret(t *testing.T) {
	yamlData := []byte(`
runs-on: host
secrets:
  - name: api_key
    from:
      env: API_KEY
  - name: api_key
    from:
      env: ANOTHER_KEY
steps:
  - cmd: echo "test"
    kind: shell
`)

	flow, err := ParseFlow(yamlData)

	require.Error(t, err)
	assert.Nil(t, flow)
	assert.ErrorIs(t, err, ErrDuplicateSecret)
}

func TestParseFlow_SecretReferenceUndeclared(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - name: deploy
    cmd: echo "Using ${{ secrets.api_key }}"
    kind: shell
`)

	flow, err := ParseFlow(yamlData)

	require.Error(t, err)
	assert.Nil(t, flow)
	assert.ErrorIs(t, err, ErrSecretNotFound)
}

func TestParseFlow_SecretReferenceFromFlowLevel(t *testing.T) {
	yamlData := []byte(`
runs-on: host
secrets:
  - name: api_key
    from:
      env: API_KEY
steps:
  - name: deploy
    cmd: echo "Using ${{ secrets.api_key }}"
    kind: shell
`)

	flow, err := ParseFlow(yamlData)

	require.NoError(t, err)
	assert.NotNil(t, flow)
}

func TestParseFlow_SecretReferenceFromStepLevel(t *testing.T) {
	yamlData := []byte(`
runs-on: host
steps:
  - name: deploy
    kind: shell
    secrets:
      - name: deploy_key
        from:
          env: DEPLOY_KEY
    cmd: echo "Using ${{ secrets.deploy_key }}"
`)

	flow, err := ParseFlow(yamlData)

	require.NoError(t, err)
	assert.NotNil(t, flow)
}
