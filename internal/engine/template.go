package engine

import (
	"fmt"
	"regexp"
	"strings"
)

// Template patterns for output and secret references.
var (
	// outputPattern matches: ${{ steps.<name>.outputs.<output> }}
	outputPattern = regexp.MustCompile(`\$\{\{\s*steps\.([a-zA-Z_][a-zA-Z0-9_-]*)\.outputs\.([a-zA-Z_][a-zA-Z0-9_-]*)\s*\}\}`)

	// secretPattern matches: ${{ secrets.<name> }}
	secretPattern = regexp.MustCompile(`\$\{\{\s*secrets\.([a-zA-Z_][a-zA-Z0-9_-]*)\s*\}\}`)
)

// TemplateRef represents a parsed output template reference.
type TemplateRef struct {
	Raw        string // full match: ${{ steps.build.outputs.version }}
	StepName   string // step name, e.g., "build"
	OutputName string // output name, e.g., "version"
}

// SecretRef represents a parsed secret template reference.
type SecretRef struct {
	Raw  string // full match: ${{ secrets.api_key }}
	Name string // secret name, e.g., "api_key"
}

// ParseTemplates finds all output template references in a string.
func ParseTemplates(input string) []TemplateRef {
	matches := outputPattern.FindAllStringSubmatch(input, -1)
	refs := make([]TemplateRef, 0, len(matches))

	for _, match := range matches {
		refs = append(refs, TemplateRef{
			Raw:        match[0],
			StepName:   match[1],
			OutputName: match[2],
		})
	}

	return refs
}

// ParseSecretTemplates finds all secret template references in a string.
func ParseSecretTemplates(input string) []SecretRef {
	matches := secretPattern.FindAllStringSubmatch(input, -1)
	refs := make([]SecretRef, 0, len(matches))

	for _, match := range matches {
		refs = append(refs, SecretRef{
			Raw:  match[0],
			Name: match[1],
		})
	}

	return refs
}

// ResolveTemplates replaces all template references with resolved values.
func ResolveTemplates(input string, state *StateManager, secrets *SecretManager) (string, error) {
	result := input

	// Resolve output references
	outputRefs := ParseTemplates(input)
	for _, ref := range outputRefs {
		value, found := state.GetOutput(ref.StepName, ref.OutputName)
		if !found {
			return "", fmt.Errorf("%w: step %q output %q not found", ErrTemplateInvalidReference, ref.StepName, ref.OutputName)
		}
		result = strings.Replace(result, ref.Raw, value, 1)
	}

	// Resolve secret references
	if secrets != nil {
		secretRefs := ParseSecretTemplates(result)
		for _, ref := range secretRefs {
			value, found := secrets.Get(ref.Name)
			if !found {
				return "", fmt.Errorf("%w: secret %q not found", ErrSecretNotFound, ref.Name)
			}
			result = strings.Replace(result, ref.Raw, value, 1)
		}
	}

	return result, nil
}

// ValidateTemplateReferences validates all template references in a flow.
// It checks that:
//   - Referenced step exists
//   - Referenced step declares that output
//   - Referenced step comes before the current step
func ValidateTemplateReferences(flow *Flow) error {
	stepIndex := make(map[string]int)
	stepOutputs := make(map[string]map[string]bool)

	for i, step := range flow.Steps {
		name := step.GetName()
		if name == "" {
			continue
		}

		stepIndex[name] = i
		stepOutputs[name] = make(map[string]bool)
		for _, output := range step.GetOutputs() {
			stepOutputs[name][output.Name] = true
		}
	}

	for i, step := range flow.Steps {
		shellStep, ok := step.(*ShellStep)
		if !ok {
			continue
		}

		refs := ParseTemplates(shellStep.Cmd)
		for _, ref := range refs {
			refStepIdx, exists := stepIndex[ref.StepName]
			if !exists {
				return fmt.Errorf("%w: step %d references non-existent step %q", ErrTemplateInvalidReference, i+1, ref.StepName)
			}

			if refStepIdx >= i {
				return fmt.Errorf("%w: step %d references step %q which is not executed before it", ErrTemplateInvalidReference, i+1, ref.StepName)
			}

			if !stepOutputs[ref.StepName][ref.OutputName] {
				return fmt.Errorf("%w: step %d references output %q from step %q which is not declared", ErrTemplateInvalidReference, i+1, ref.OutputName, ref.StepName)
			}
		}
	}

	return nil
}

// ValidateSecretReferences validates all secret references in a flow.
// It checks that referenced secrets are declared at flow or step level.
func ValidateSecretReferences(flow *Flow) error {
	// Build set of flow-level secret names
	flowSecrets := make(map[string]bool)
	for _, secret := range flow.Secrets {
		flowSecrets[secret.Name] = true
	}

	for i, step := range flow.Steps {
		shellStep, ok := step.(*ShellStep)
		if !ok {
			continue
		}

		// Build set of step-level secret names
		stepSecrets := make(map[string]bool)
		for _, secret := range shellStep.Secrets {
			stepSecrets[secret.Name] = true
		}

		// Check all secret references
		refs := ParseSecretTemplates(shellStep.Cmd)
		for _, ref := range refs {
			if !flowSecrets[ref.Name] && !stepSecrets[ref.Name] {
				return fmt.Errorf("%w: step %d references undeclared secret %q", ErrSecretNotFound, i+1, ref.Name)
			}
		}
	}

	return nil
}
