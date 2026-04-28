//go:generate go-enum --marshal --mustparse --values --names --output-suffix _generated

package engine

// ENUM(json, console)
type ConfigLogEncoding string

// ENUM(epoch, epochmillis, epochnanos, iso8601, rfc3339, rfc3339nano)
type ConfigLogTimeEncoder string
