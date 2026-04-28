//go:generate go-enum --marshal --mustparse --values --names --output-suffix _generated

package engine

// ENUM(host)
type RunsOnKind string

// ENUM(shell)
type StepKind string

// ENUM(variable, file)
type OutputType string
