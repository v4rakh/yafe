package engine

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// rawFlow is the intermediate YAML parsing struct.
type rawFlow struct {
	StateDir string              `yaml:"state-dir"`
	Tools    []rawTool           `yaml:"tools"`
	Secrets  []SecretDeclaration `yaml:"secrets"`
	RunsOn   string              `yaml:"runs-on"`
	Steps    []rawStep           `yaml:"steps"`
}

// rawTool is the intermediate struct for tool parsing.
// Uses pointer for Retries to distinguish between "not specified" and "explicitly 0".
type rawTool struct {
	Name    string `yaml:"name"`
	URL     string `yaml:"url"`
	Path    string `yaml:"path"`
	SHA256  string `yaml:"sha256"`
	Retries *int   `yaml:"retries"`
}

// rawStep is the intermediate struct for step parsing.
// It contains all fields from all step types.
type rawStep struct {
	Kind    string              `yaml:"kind"`
	Name    string              `yaml:"name"`
	Cmd     string              `yaml:"cmd"`
	Shell   string              `yaml:"shell"`
	Env     []string            `yaml:"env"`
	Outputs []StepOutput        `yaml:"outputs"`
	Secrets []SecretDeclaration `yaml:"secrets"`
}

// ParseFlow parses YAML data into a Flow struct.
func ParseFlow(data []byte) (*Flow, error) {
	var raw rawFlow
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid yaml contents: %w", err)
	}

	flow, err := convertRawFlow(&raw)
	if err != nil {
		return nil, err
	}

	if err := ValidateFlow(flow); err != nil {
		return nil, err
	}

	return flow, nil
}

// convertRawFlow converts the intermediate rawFlow to a Flow.
func convertRawFlow(raw *rawFlow) (*Flow, error) {
	flow := &Flow{
		StateDir: raw.StateDir,
		Secrets:  raw.Secrets,
	}

	// Convert tools with default retries
	for _, rawTool := range raw.Tools {
		tool := ToolDeclaration{
			Name:   rawTool.Name,
			URL:    rawTool.URL,
			Path:   rawTool.Path,
			SHA256: rawTool.SHA256,
		}
		if rawTool.Retries != nil {
			tool.Retries = *rawTool.Retries
		} else {
			tool.Retries = defaultRetries
		}
		flow.Tools = append(flow.Tools, tool)
	}

	// Parse runs-on
	if raw.RunsOn == "" {
		return nil, fmt.Errorf("runs-on: must be provided")
	}
	switch raw.RunsOn {
	case RunsOnKindHost.String():
		flow.RunsOn = &RunsOnHost{kind: RunsOnKindHost}
	default:
		return nil, fmt.Errorf("runs-on: unknown %s, must be one of %+v", raw.RunsOn, RunsOnKindNames())
	}

	// Parse steps
	if len(raw.Steps) == 0 {
		return nil, fmt.Errorf("steps: must be provided")
	}

	flow.Steps = make([]Step, len(raw.Steps))
	for i, rawStep := range raw.Steps {
		step, err := convertRawStep(i, &rawStep)
		if err != nil {
			return nil, err
		}
		flow.Steps[i] = step
	}

	return flow, nil
}

// convertRawStep converts a rawStep to a Step interface.
func convertRawStep(index int, raw *rawStep) (Step, error) {
	if raw.Kind == "" {
		return nil, fmt.Errorf("step %d: missing kind field", index)
	}

	switch StepKind(raw.Kind) {
	case StepKindShell:
		step := &ShellStep{
			Name:    raw.Name,
			Cmd:     raw.Cmd,
			Shell:   raw.Shell,
			Env:     raw.Env,
			Outputs: raw.Outputs,
			Secrets: raw.Secrets,
		}
		step.EnsureDefaults()
		return step, nil
	default:
		return nil, fmt.Errorf("step %d: unknown kind %s, must be one of %+v", index, raw.Kind, StepKindNames())
	}
}

// ValidateFlow validates the flow configuration.
func ValidateFlow(flow *Flow) error {
	// Validate tools
	if err := validateTools(flow.Tools); err != nil {
		return err
	}

	// Validate flow-level secrets
	if err := validateSecrets("flow", flow.Secrets); err != nil {
		return err
	}

	stepNames := make(map[string]int)
	hasTemplates := false

	for i, step := range flow.Steps {
		name := step.GetName()
		outputs := step.GetOutputs()
		secrets := step.GetSecrets()

		if name != "" {
			if prevIdx, exists := stepNames[name]; exists {
				return fmt.Errorf("%w: %q used in step %d and step %d", ErrDuplicateStepName, name, prevIdx+1, i+1)
			}
			stepNames[name] = i
		}

		for j, output := range outputs {
			if output.Name == "" {
				return fmt.Errorf("step %d output %d: name is required", i+1, j+1)
			}

			if !output.Type.IsValid() {
				return fmt.Errorf("step %d output %d: %w", i+1, j+1, ErrInvalidOutputType)
			}

			if output.Type == OutputTypeFile && output.Path == "" {
				return fmt.Errorf("%w: step %d output %d", ErrMissingOutputPath, i+1, j+1)
			}
		}

		if len(outputs) > 0 && name == "" {
			return fmt.Errorf("step %d: steps with outputs must have a name", i+1)
		}

		// Validate step-level secrets
		if err := validateSecrets(fmt.Sprintf("step %d", i+1), secrets); err != nil {
			return err
		}

		if shellStep, ok := step.(*ShellStep); ok {
			refs := ParseTemplates(shellStep.Cmd)
			if len(refs) > 0 {
				hasTemplates = true
			}
		}
	}

	if hasTemplates {
		if err := ValidateTemplateReferences(flow); err != nil {
			return err
		}
	}

	// Validate secret references
	if err := ValidateSecretReferences(flow); err != nil {
		return err
	}

	return nil
}

// validateSecrets validates a list of secret declarations.
func validateSecrets(scope string, secrets []SecretDeclaration) error {
	seen := make(map[string]bool)
	for i, secret := range secrets {
		if secret.Name == "" {
			return fmt.Errorf("%s secret %d: name is required", scope, i+1)
		}

		if seen[secret.Name] {
			return fmt.Errorf("%w: %q in %s", ErrDuplicateSecret, secret.Name, scope)
		}
		seen[secret.Name] = true

		if secret.From.Env == "" && secret.From.File == "" {
			return fmt.Errorf("%w: %s secret %q", ErrSecretSourceEmpty, scope, secret.Name)
		}
	}
	return nil
}

// validateTools validates the tools declarations.
func validateTools(tools []ToolDeclaration) error {
	seen := make(map[string]bool)

	for i, tool := range tools {
		if tool.Name == "" {
			return fmt.Errorf("tool %d: %w", i+1, ErrToolNameRequired)
		}

		if seen[tool.Name] {
			return fmt.Errorf("%w: %q", ErrDuplicateTool, tool.Name)
		}
		seen[tool.Name] = true

		if tool.Retries < 1 {
			return fmt.Errorf("tool %q: %w", tool.Name, ErrToolInvalidRetries)
		}

		// Archive URLs require path
		if tool.URL != "" && isArchiveURL(tool.URL) && tool.Path == "" {
			return fmt.Errorf("tool %q: %w", tool.Name, ErrToolPathRequired)
		}

		// Non-archive URLs forbid path
		if tool.URL != "" && !isArchiveURL(tool.URL) && tool.Path != "" {
			return fmt.Errorf("tool %q: %w", tool.Name, ErrToolPathForbidden)
		}
	}

	return nil
}
