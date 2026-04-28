package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTemplates_SingleReference(t *testing.T) {
	input := `echo "Deploying ${{ steps.build.outputs.version }}"`

	refs := ParseTemplates(input)

	require.Len(t, refs, 1)
	assert.Equal(t, "${{ steps.build.outputs.version }}", refs[0].Raw)
	assert.Equal(t, "build", refs[0].StepName)
	assert.Equal(t, "version", refs[0].OutputName)
}

func TestParseTemplates_MultipleReferences(t *testing.T) {
	input := `echo "${{ steps.build.outputs.version }}" && unzip ${{ steps.build.outputs.artifact }}`

	refs := ParseTemplates(input)

	require.Len(t, refs, 2)
	assert.Equal(t, "build", refs[0].StepName)
	assert.Equal(t, "version", refs[0].OutputName)
	assert.Equal(t, "build", refs[1].StepName)
	assert.Equal(t, "artifact", refs[1].OutputName)
}

func TestParseTemplates_NoReferences(t *testing.T) {
	input := `echo "Hello world"`

	refs := ParseTemplates(input)

	assert.Empty(t, refs)
}

func TestParseTemplates_WithSpaces(t *testing.T) {
	input := `echo "${{  steps.build.outputs.version  }}"`

	refs := ParseTemplates(input)

	require.Len(t, refs, 1)
	assert.Equal(t, "build", refs[0].StepName)
	assert.Equal(t, "version", refs[0].OutputName)
}

func TestParseTemplates_StepNameWithHyphen(t *testing.T) {
	input := `echo "${{ steps.build-app.outputs.version }}"`

	refs := ParseTemplates(input)

	require.Len(t, refs, 1)
	assert.Equal(t, "build-app", refs[0].StepName)
	assert.Equal(t, "version", refs[0].OutputName)
}

func TestParseTemplates_StepNameWithUnderscore(t *testing.T) {
	input := `echo "${{ steps.build_app.outputs.version_number }}"`

	refs := ParseTemplates(input)

	require.Len(t, refs, 1)
	assert.Equal(t, "build_app", refs[0].StepName)
	assert.Equal(t, "version_number", refs[0].OutputName)
}

func TestParseSecretTemplates_SingleReference(t *testing.T) {
	input := `echo "${{ secrets.api_key }}"`

	refs := ParseSecretTemplates(input)

	require.Len(t, refs, 1)
	assert.Equal(t, "${{ secrets.api_key }}", refs[0].Raw)
	assert.Equal(t, "api_key", refs[0].Name)
}

func TestParseSecretTemplates_MultipleReferences(t *testing.T) {
	input := `curl -H "Authorization: ${{ secrets.token }}" -u "${{ secrets.user }}:${{ secrets.pass }}"`

	refs := ParseSecretTemplates(input)

	require.Len(t, refs, 3)
	assert.Equal(t, "token", refs[0].Name)
	assert.Equal(t, "user", refs[1].Name)
	assert.Equal(t, "pass", refs[2].Name)
}

func TestResolveTemplates_OutputsOnly(t *testing.T) {
	state := NewStateManager("", "test-run")
	state.SetOutput("build", "version", "1.0.0")

	input := `echo "Deploying ${{ steps.build.outputs.version }}"`
	result, err := ResolveTemplates(input, state, nil)

	require.NoError(t, err)
	assert.Equal(t, `echo "Deploying 1.0.0"`, result)
}

func TestResolveTemplates_MultipleOutputs(t *testing.T) {
	state := NewStateManager("", "test-run")
	state.SetOutput("build", "version", "1.0.0")
	state.SetOutput("build", "artifact", "/tmp/build/artifact.zip")

	input := `echo "${{ steps.build.outputs.version }}" && unzip ${{ steps.build.outputs.artifact }}`
	result, err := ResolveTemplates(input, state, nil)

	require.NoError(t, err)
	assert.Equal(t, `echo "1.0.0" && unzip /tmp/build/artifact.zip`, result)
}

func TestResolveTemplates_SecretsOnly(t *testing.T) {
	state := NewStateManager("", "test-run")
	secrets := NewSecretManager()
	secrets.Set("api_key", "secret123")

	input := `curl -H "Authorization: ${{ secrets.api_key }}"`
	result, err := ResolveTemplates(input, state, secrets)

	require.NoError(t, err)
	assert.Equal(t, `curl -H "Authorization: secret123"`, result)
}

func TestResolveTemplates_OutputsAndSecrets(t *testing.T) {
	state := NewStateManager("", "test-run")
	state.SetOutput("build", "version", "1.0.0")

	secrets := NewSecretManager()
	secrets.Set("api_key", "secret123")

	input := `curl -H "Authorization: ${{ secrets.api_key }}" https://api.example.com/${{ steps.build.outputs.version }}`
	result, err := ResolveTemplates(input, state, secrets)

	require.NoError(t, err)
	assert.Equal(t, `curl -H "Authorization: secret123" https://api.example.com/1.0.0`, result)
}

func TestResolveTemplates_MissingOutput(t *testing.T) {
	state := NewStateManager("", "test-run")

	input := `echo "${{ steps.build.outputs.version }}"`
	_, err := ResolveTemplates(input, state, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTemplateInvalidReference)
}

func TestResolveTemplates_MissingSecret(t *testing.T) {
	state := NewStateManager("", "test-run")
	secrets := NewSecretManager()

	input := `echo "${{ secrets.api_key }}"`
	_, err := ResolveTemplates(input, state, secrets)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSecretNotFound)
}

func TestValidateTemplateReferences_Valid(t *testing.T) {
	flow := &Flow{
		Steps: []Step{
			&ShellStep{
				Name: "build",
				Cmd:  `echo "version=1.0.0" >> $YAFE_OUTPUT`,
				Outputs: []StepOutput{
					{Name: "version", Type: OutputTypeVariable},
				},
			},
			&ShellStep{
				Name: "deploy",
				Cmd:  `echo "Deploying ${{ steps.build.outputs.version }}"`,
			},
		},
	}

	err := ValidateTemplateReferences(flow)

	assert.NoError(t, err)
}

func TestValidateTemplateReferences_NonExistentStep(t *testing.T) {
	flow := &Flow{
		Steps: []Step{
			&ShellStep{
				Name: "deploy",
				Cmd:  `echo "Deploying ${{ steps.build.outputs.version }}"`,
			},
		},
	}

	err := ValidateTemplateReferences(flow)

	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrTemplateInvalidReference)
	assert.Contains(t, err.Error(), "non-existent step")
}

func TestValidateTemplateReferences_StepNotExecutedBefore(t *testing.T) {
	flow := &Flow{
		Steps: []Step{
			&ShellStep{
				Name: "deploy",
				Cmd:  `echo "Deploying ${{ steps.build.outputs.version }}"`,
			},
			&ShellStep{
				Name: "build",
				Cmd:  `echo "version=1.0.0" >> $YAFE_OUTPUT`,
				Outputs: []StepOutput{
					{Name: "version", Type: OutputTypeVariable},
				},
			},
		},
	}

	err := ValidateTemplateReferences(flow)

	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrTemplateInvalidReference)
	assert.Contains(t, err.Error(), "not executed before")
}

func TestValidateTemplateReferences_UndeclaredOutput(t *testing.T) {
	flow := &Flow{
		Steps: []Step{
			&ShellStep{
				Name: "build",
				Cmd:  `echo "version=1.0.0" >> $YAFE_OUTPUT`,
			},
			&ShellStep{
				Name: "deploy",
				Cmd:  `echo "Deploying ${{ steps.build.outputs.version }}"`,
			},
		},
	}

	err := ValidateTemplateReferences(flow)

	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrTemplateInvalidReference)
	assert.Contains(t, err.Error(), "not declared")
}

func TestValidateSecretReferences_Valid(t *testing.T) {
	flow := &Flow{
		Secrets: []SecretDeclaration{
			{Name: "api_key", From: SecretSource{Env: "API_KEY"}},
		},
		Steps: []Step{
			&ShellStep{
				Name: "deploy",
				Cmd:  `curl -H "Authorization: ${{ secrets.api_key }}"`,
			},
		},
	}

	err := ValidateSecretReferences(flow)

	assert.NoError(t, err)
}

func TestValidateSecretReferences_StepLevelSecret(t *testing.T) {
	flow := &Flow{
		Steps: []Step{
			&ShellStep{
				Name: "deploy",
				Secrets: []SecretDeclaration{
					{Name: "deploy_key", From: SecretSource{Env: "DEPLOY_KEY"}},
				},
				Cmd: `curl -H "Authorization: ${{ secrets.deploy_key }}"`,
			},
		},
	}

	err := ValidateSecretReferences(flow)

	assert.NoError(t, err)
}

func TestValidateSecretReferences_UndeclaredSecret(t *testing.T) {
	flow := &Flow{
		Steps: []Step{
			&ShellStep{
				Name: "deploy",
				Cmd:  `curl -H "Authorization: ${{ secrets.api_key }}"`,
			},
		},
	}

	err := ValidateSecretReferences(flow)

	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrSecretNotFound)
}
