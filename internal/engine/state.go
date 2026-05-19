package engine

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

// StateManager manages state directory and output collection for workflow steps.
type StateManager struct {
	baseDir     string
	runID       string
	outputs     map[string]map[string]string // step -> output -> value/path
	inputs      map[string]string            // job inputs as YAFE_INPUT_* env vars
	logWriter   io.Writer
	toolManager *ToolManager
}

// NewStateManager creates a new state manager.
// If baseDir is empty, it defaults to the system temp directory with a yafe-<runID> prefix.
func NewStateManager(baseDir string, runID string) *StateManager {
	if baseDir == "" {
		baseDir = filepath.Join(os.TempDir(), stateDirPrefix+runID)
	}
	return &StateManager{
		baseDir: baseDir,
		runID:   runID,
		outputs: make(map[string]map[string]string),
	}
}

// BaseDir returns the absolute path to the state directory.
func (s *StateManager) BaseDir() string {
	return s.baseDir
}

// RunID returns the run identifier.
func (s *StateManager) RunID() string {
	return s.runID
}

// SetLogWriter sets the writer for step output logs.
func (s *StateManager) SetLogWriter(w io.Writer) {
	s.logWriter = w
}

// LogWriter returns the log writer, defaulting to os.Stdout if not set.
func (s *StateManager) LogWriter() io.Writer {
	if s.logWriter != nil {
		return s.logWriter
	}
	return os.Stdout
}

// SetToolManager sets the tool manager for this state.
func (s *StateManager) SetToolManager(tm *ToolManager) {
	s.toolManager = tm
}

// ToolManager returns the tool manager.
func (s *StateManager) ToolManager() *ToolManager {
	return s.toolManager
}

// SetInputs sets the job inputs to be exposed as YAFE_INPUT_* environment variables.
func (s *StateManager) SetInputs(inputs map[string]string) {
	s.inputs = inputs
}

// Inputs returns the job inputs.
func (s *StateManager) Inputs() map[string]string {
	return s.inputs
}

// Initialize creates the state directory structure.
func (s *StateManager) Initialize() error {
	if err := os.MkdirAll(s.baseDir, dirMode); err != nil {
		return fmt.Errorf("creating state directory %s: %w", s.baseDir, err)
	}

	outputsDir := filepath.Join(s.baseDir, outputsDirName)
	if err := os.MkdirAll(outputsDir, dirMode); err != nil {
		return fmt.Errorf("creating outputs directory %s: %w", outputsDir, err)
	}

	log.Debug().Msgf("Initialized state directory at %s", s.baseDir)
	return nil
}

// StepDir returns the path to a step's directory within the state directory.
func (s *StateManager) StepDir(stepName string) string {
	return filepath.Join(s.baseDir, stepName)
}

// OutputFile returns the path to a step's output file (for key=value pairs).
func (s *StateManager) OutputFile(stepName string) string {
	return filepath.Join(s.baseDir, outputsDirName, stepName)
}

// PrepareStep creates the step's output directory and output file.
func (s *StateManager) PrepareStep(stepName string) error {
	stepDir := s.StepDir(stepName)
	if err := os.MkdirAll(stepDir, dirMode); err != nil {
		return fmt.Errorf("creating step directory %s: %w", stepDir, err)
	}

	outputFile := s.OutputFile(stepName)
	//gosec:disable G304 -- Explicitly allowed to use any location
	f, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("creating output file %s: %w", outputFile, err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("closing output file %s: %w", outputFile, err)
	}

	return nil
}

// CollectOutputs parses the output file and verifies declared outputs exist.
func (s *StateManager) CollectOutputs(step Step) error {
	stepName := step.GetName()
	outputs := step.GetOutputs()

	if len(outputs) == 0 {
		return nil
	}

	if s.outputs[stepName] == nil {
		s.outputs[stepName] = make(map[string]string)
	}

	variables, err := s.parseOutputFile(stepName)
	if err != nil {
		return err
	}

	for _, output := range outputs {
		switch output.Type {
		case OutputTypeVariable:
			value, found := variables[output.Name]
			if !found {
				return fmt.Errorf("step %q did not produce variable output %q", stepName, output.Name)
			}
			s.outputs[stepName][output.Name] = value
			log.Debug().Msgf("Collected variable output %s.%s = %s", stepName, output.Name, value)

		case OutputTypeFile:
			fullPath := filepath.Join(s.baseDir, output.Path)
			cleanBase := filepath.Clean(s.baseDir) + string(filepath.Separator)
			cleanFull := filepath.Clean(fullPath)
			if !strings.HasPrefix(cleanFull, cleanBase) && cleanFull != filepath.Clean(s.baseDir) {
				return fmt.Errorf("%w: %s escapes state directory", ErrPathTraversal, output.Path)
			}
			if _, err := os.Stat(fullPath); err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("step %q did not produce file %q at %s", stepName, output.Name, fullPath)
				}
				return fmt.Errorf("checking file output %q: %w", output.Name, err)
			}
			s.outputs[stepName][output.Name] = fullPath
			log.Debug().Msgf("Collected file output %s.%s = %s", stepName, output.Name, fullPath)

		default:
			return fmt.Errorf("%s is %w", output.Type, ErrInvalidOutputType)
		}
	}

	return nil
}

// parseOutputFile reads key=value pairs from a step's output file.
func (s *StateManager) parseOutputFile(stepName string) (map[string]string, error) {
	result := make(map[string]string)
	outputFile := s.OutputFile(stepName)

	//gosec:disable G304 -- Explicitly allowed to use any location
	file, err := os.Open(outputFile)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, fmt.Errorf("opening output file %s: %w", outputFile, err)
	}
	defer file.Close() //nolint:errcheck

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			result[key] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading output file %s: %w", outputFile, err)
	}

	return result, nil
}

// GetOutput retrieves an output value from a step.
func (s *StateManager) GetOutput(stepName, outputName string) (string, bool) {
	stepOutputs, ok := s.outputs[stepName]
	if !ok {
		return "", false
	}
	value, ok := stepOutputs[outputName]
	return value, ok
}

// SetOutput sets an output value for a step. Primarily used for testing.
func (s *StateManager) SetOutput(stepName, outputName, value string) {
	if s.outputs[stepName] == nil {
		s.outputs[stepName] = make(map[string]string)
	}
	s.outputs[stepName][outputName] = value
}

// Cleanup removes the entire state directory.
func (s *StateManager) Cleanup() error {
	log.Debug().Msgf("Cleaning up state directory at %s", s.baseDir)
	if err := os.RemoveAll(s.baseDir); err != nil {
		return fmt.Errorf("removing state directory %s: %w", s.baseDir, err)
	}
	return nil
}
