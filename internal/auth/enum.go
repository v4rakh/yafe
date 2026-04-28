//go:generate go-enum --marshal --mustparse --values --names --output-suffix _generated

package auth

// ENUM(jobs:read, jobs:write, flows:read, flows:write, schedules:read, schedules:write)
type Role string
