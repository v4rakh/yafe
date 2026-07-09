package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

// RunOption configures flow execution.
type RunOption func(*runOptions)

type runOptions struct {
	logWriter io.Writer
	inputs    map[string]string
}

// WithLogWriter sets the writer for step stdout/stderr output.
func WithLogWriter(w io.Writer) RunOption {
	return func(o *runOptions) {
		o.logWriter = w
	}
}

// WithInputs sets input variables available as YAFE_INPUT_* environment variables.
func WithInputs(inputs map[string]string) RunOption {
	return func(o *runOptions) {
		o.inputs = inputs
	}
}

// Step represents runtime stages.
type Step interface {
	Kind() StepKind
	Execute(ctx context.Context, runID string, state *StateManager) error
	EnsureDefaults()
	GetName() string
	GetOutputs() []StepOutput
	GetSecrets() []SecretDeclaration
	// ResolveTemplates returns a copy of the step with templates resolved.
	ResolveTemplates(state *StateManager, secrets *SecretManager) (Step, error)
}

// generateRunID creates a random run identifier.
// Panics if crypto/rand fails, as this indicates a system-level issue.
func generateRunID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// RunsOn represents the environment steps are executed on
type RunsOn interface {
	Kind() RunsOnKind
	Bootstrap() error
	Teardown() error
	Supports(step Step) bool
}

// Flow runtime structure.
type Flow struct {
	StateDir string              `yaml:"state-dir"` // optional, defaults to /tmp/yafe-<runID>
	Tools    []ToolDeclaration   `yaml:"tools"`     // tool dependencies
	Secrets  []SecretDeclaration `yaml:"secrets"`   // flow-level secrets
	RunsOn   RunsOn              `yaml:"runs-on"`
	Steps    []Step              `yaml:"steps"`
}

// Executor defines the interface for flow execution.
type Executor interface {
	LoadFromFile(filename string) (*Flow, error)
	Run(ctx context.Context, flow *Flow, opts ...RunOption) error
}

// Engine can load and run a Flow with LoadFromFile or LoadBytes.
type Engine struct{}

// NewEngine creates a new engine.
func NewEngine() *Engine {
	return &Engine{}
}

// LoadFromFile loads a Flow from file
func (e *Engine) LoadFromFile(filename string) (*Flow, error) {
	//gosec:disable G304 -- Explicitly allowed to use any location
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrLoadRead, filename, err)
	}

	return e.LoadBytes(data)
}

// LoadBytes loads a Flow from plain bytes
func (e *Engine) LoadBytes(data []byte) (*Flow, error) {
	flow, err := ParseFlow(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLoadParsing, err)
	}
	return flow, nil
}

// Run executes provided Flow through this Engine.
func (e *Engine) Run(ctx context.Context, flow *Flow, opts ...RunOption) error {
	if flow == nil || flow.RunsOn == nil {
		return ErrValidationNotNil
	}

	// Apply options
	var options runOptions
	for _, opt := range opts {
		opt(&options)
	}

	runID := generateRunID()
	log.Debug().Msgf("Executing flow with run ID '%s'", runID)

	// Initialize state manager
	state := NewStateManager(flow.StateDir, runID)
	if options.logWriter != nil {
		state.SetLogWriter(options.logWriter)
	}
	if options.inputs != nil {
		state.SetInputs(options.inputs)
	}
	if err := state.Initialize(); err != nil {
		return fmt.Errorf("%w: %w", ErrStateInitialize, err)
	}
	defer func() {
		if err := state.Cleanup(); err != nil {
			log.Error().Err(err).Msg("State cleanup failed")
		}
	}()

	// Resolve tools before any step execution
	if len(flow.Tools) > 0 {
		toolManager := NewToolManager(filepath.Join(state.BaseDir(), toolsDirName))
		if err := toolManager.Initialize(); err != nil {
			return fmt.Errorf("initializing tools: %w", err)
		}
		if err := toolManager.Resolve(flow.Tools); err != nil {
			return fmt.Errorf("resolving tools: %w", err)
		}
		state.SetToolManager(toolManager)
	}

	// Load flow-level secrets
	flowSecrets := NewSecretManager()
	if err := flowSecrets.Load(flow.Secrets); err != nil {
		return fmt.Errorf("loading flow secrets: %w", err)
	}
	defer flowSecrets.Clear()

	if err := flow.RunsOn.Bootstrap(); err != nil {
		return fmt.Errorf("%w: %w", ErrRunsOnBootstrap, err)
	}
	defer func(RunsOn RunsOn) {
		err := RunsOn.Teardown()
		if err != nil {
			log.Error().Err(fmt.Errorf("%w: %w", ErrRunsOnTeardown, err)).Msg("RunsOn teardown failed")
		}
	}(flow.RunsOn)

	for i, step := range flow.Steps {
		// Check for cancellation before each step
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%w: at step %d: %w", ErrStepExecution, i+1, err)
		}

		if !flow.RunsOn.Supports(step) {
			return fmt.Errorf("%w: step %d", ErrStepIncompatible, i+1)
		}

		// Build step secrets: flow-level + step-level (step overrides flow)
		stepSecrets := flowSecrets.Clone()
		if err := stepSecrets.Load(step.GetSecrets()); err != nil {
			return fmt.Errorf("%w: at step %d: loading secrets: %w", ErrStepExecution, i+1, err)
		}

		// Resolve templates in step before execution
		resolvedStep, err := step.ResolveTemplates(state, stepSecrets)
		if err != nil {
			return fmt.Errorf("%w: at step %d: %w", ErrStepExecution, i+1, err)
		}

		// Prepare state directory for this step
		stepName := step.GetName()
		if stepName != "" {
			if err = state.PrepareStep(stepName); err != nil {
				return fmt.Errorf("%w: at step %d: %w", ErrStepExecution, i+1, err)
			}
		}

		log.Debug().Msgf("Executing step %d/%d", i+1, len(flow.Steps))
		if err = resolvedStep.Execute(ctx, runID, state); err != nil {
			return fmt.Errorf("%w: at step %d: %s", ErrStepExecution, i+1, stepSecrets.Mask(err.Error()))
		}

		// Collect outputs after successful execution
		if err = state.CollectOutputs(step); err != nil {
			return fmt.Errorf("%w: at step %d: %w", ErrOutputNotProduced, i+1, err)
		}
	}

	log.Debug().Msg("Flow execution completed")
	return nil
}
