package engine

import (
	"github.com/rs/zerolog/log"
)

// RunsOnHost host execution RunsOn directive
type RunsOnHost struct {
	kind RunsOnKind
}

// Kind returns the kind
func (r *RunsOnHost) Kind() RunsOnKind { return RunsOnKindHost }

// Bootstrap bootstraps necessary prerequisites
func (r *RunsOnHost) Bootstrap() error {
	log.Debug().Msg("Bootstrapping host runs-on...")
	return nil
}

// Teardown tears down runs on
func (r *RunsOnHost) Teardown() error {
	log.Debug().Msg("Tearing down host runs-on...")
	return nil
}

// Supports true if step is supported on this RunsOn, false otherwise
func (r *RunsOnHost) Supports(step Step) bool {
	_, ok := step.(*ShellStep)
	return ok
}
