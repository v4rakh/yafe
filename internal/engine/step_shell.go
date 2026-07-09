package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/rs/zerolog/log"
)

// StepOutput represents a declared output from a step.
type StepOutput struct {
	Name string     `yaml:"name"` // required, output identifier
	Type OutputType `yaml:"type"` // variable or file
	Path string     `yaml:"path"` // required if type=file, relative to state-dir
}

// ShellStep steps for shell execution.
type ShellStep struct {
	Name    string              `yaml:"name"`              // required for state sharing, unique identifier
	Cmd     string              `yaml:"cmd"`               // command to execute
	Shell   string              `yaml:"shell,omitempty"`   // shell to use (default: bash)
	Env     []string            `yaml:"env,omitempty"`     // environment variables
	Outputs []StepOutput        `yaml:"outputs,omitempty"` // declared outputs
	Secrets []SecretDeclaration `yaml:"secrets,omitempty"` // step-level secrets
}

// Kind returns the kind
func (s *ShellStep) Kind() StepKind {
	return StepKindShell
}

// GetName returns the step name
func (s *ShellStep) GetName() string {
	return s.Name
}

// GetOutputs returns the step outputs.
func (s *ShellStep) GetOutputs() []StepOutput {
	return s.Outputs
}

// GetSecrets returns the step secrets.
func (s *ShellStep) GetSecrets() []SecretDeclaration {
	return s.Secrets
}

// EnsureDefaults injects proper defaults into ShellStep struct.
func (s *ShellStep) EnsureDefaults() {
	if s.Shell == "" {
		s.Shell = defaultShell
	}
}

// ResolveTemplates returns a copy of the step with templates resolved.
func (s *ShellStep) ResolveTemplates(state *StateManager, secrets *SecretManager) (Step, error) {
	resolvedCmd, err := ResolveTemplates(s.Cmd, state, secrets)
	if err != nil {
		return nil, err
	}

	resolved := *s
	resolved.Cmd = resolvedCmd
	return &resolved, nil
}

// Execute runs a step.
func (s *ShellStep) Execute(ctx context.Context, runID string, state *StateManager) error {
	if strings.HasPrefix(strings.TrimSpace(s.Cmd), "#") {
		return nil
	}

	prefix := fmt.Sprintf(scriptPrefix, os.Getpid(), runID)
	scriptFile, err := os.CreateTemp("", prefix+scriptSuffix)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrShellStepTempScriptCreation, err)
	}
	scriptPath := scriptFile.Name()

	defer func() {
		if errRemove := os.Remove(scriptPath); errRemove != nil {
			log.Warn().Err(errRemove).Msgf("Cleanup for temporary shell step script '%s' failed", scriptPath)
		} else {
			log.Debug().Msgf("Cleaned up temporary shell step script '%s'", scriptPath)
		}
	}()

	defer func(f *os.File) {
		if errClose := f.Close(); errClose != nil {
			log.Error().Err(errClose).Msg("Close temporary shell step script")
		}
	}(scriptFile)

	scriptContent := fmt.Sprintf("#!%s %s\nset -euo pipefail\n%s\n", shellShebang, s.Shell, strings.TrimSpace(s.Cmd))

	if err = scriptFile.Chmod(scriptFileMode); err != nil {
		return fmt.Errorf("%w for '%s': %w", ErrShellStepTempScriptExecutable, scriptPath, err)
	}
	if _, err = scriptFile.WriteString(scriptContent); err != nil {
		return fmt.Errorf("%w for '%s': %w", ErrShellStepTempScriptIO, scriptPath, err)
	}
	if err = scriptFile.Sync(); err != nil {
		return fmt.Errorf("%w for '%s': %w", ErrShellStepTempScriptIO, scriptPath, err)
	}

	log.Debug().Msgf("Created temporary shell step script '%s'", scriptPath)

	//gosec:disable G204 G702 -- False positive - shellShebang/shell/scriptPath are trusted variables or explicitly given by user for a step
	cmd := exec.CommandContext(ctx, shellShebang, s.Shell, scriptPath)
	if state != nil {
		cmd.Dir = state.BaseDir()
		cmd.Stdout = state.LogWriter()
		cmd.Stderr = state.LogWriter()
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	cmd.Stdin = os.Stdin
	cmd.Env = append(os.Environ(), s.Env...)

	// Prepend tools directory to PATH
	if state != nil && state.ToolManager() != nil {
		tm := state.ToolManager()
		for i, env := range cmd.Env {
			if strings.HasPrefix(env, "PATH=") {
				cmd.Env[i] = "PATH=" + tm.ToolsDir() + string(os.PathListSeparator) + env[5:]
				break
			}
		}
	}

	// Add job inputs as YAFE_INPUT_* environment variables
	if state != nil {
		for key, value := range state.Inputs() {
			cmd.Env = append(cmd.Env, fmt.Sprintf("YAFE_INPUT_%s=%s", strings.ToUpper(key), value))
		}
	}

	if state != nil && s.Name != "" {
		cmd.Env = append(cmd.Env,
			fmt.Sprintf("%s=%s", EnvYafeState, state.BaseDir()),
			fmt.Sprintf("%s=%s", EnvYafeOutput, state.OutputFile(s.Name)),
		)
	}

	log.Debug().Msgf("Executing temporary shell step '%s' using '%s'", scriptPath, s.Shell)

	if err = cmd.Run(); err != nil {
		return fmt.Errorf("%w for '%s': %w", ErrShellStepExecution, scriptPath, err)
	}

	return nil
}
