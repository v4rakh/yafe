//go:generate go-enum --marshal --mustparse --values --names --output-suffix _generated

package scheduler

// ENUM(cron, interval)
type ScheduleType string
