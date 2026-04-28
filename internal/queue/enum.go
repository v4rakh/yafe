//go:generate go-enum --marshal --mustparse --values --names --output-suffix _generated

package queue

// ENUM(pending, running, done, failed)
type JobStatus string
