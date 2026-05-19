package engine

import (
	"context"
	golog "log"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sethvargo/go-envconfig"
)

type ConfigLogging struct {
	Encoding              ConfigLogEncoding    `env:"LOGGING_ENCODING,default=console"                   validate:"required,oneof=json console"`
	EncodingColorize      bool                 `env:"LOGGING_ENCODING_COLORIZE,default=true"`
	EncodingErrorKey      string               `env:"LOGGING_ENCODING_ERROR_KEY,default=error"           validate:"required"`
	EncodingFileKey       string               `env:"LOGGING_ENCODING_FILE_KEY,default=file"             validate:"required"`
	EncodingFuncKey       string               `env:"LOGGING_ENCODING_FUNC_KEY,default=func"             validate:"required"`
	EncodingLevelKey      string               `env:"LOGGING_ENCODING_LEVEL_KEY,default=level"           validate:"required"`
	EncodingMessageKey    string               `env:"LOGGING_ENCODING_MESSAGE_KEY,default=msg"           validate:"required"`
	EncodingStacktraceKey string               `env:"LOGGING_ENCODING_STACKTRACE_KEY,default=stacktrace" validate:"required"`
	EncodingTimeEncoder   ConfigLogTimeEncoder `env:"LOGGING_ENCODING_TIME_ENCODER,default=iso8601"      validate:"required,oneof=epoch epochmillis epochnanos iso8601 rfc3339 rfc3339nano"`
	EncodingTimeKey       string               `env:"LOGGING_ENCODING_TIME_KEY,default=ts"               validate:"required"`
	Level                 string               `env:"LOGGING_LEVEL,default=info"                         validate:"required,oneof=trace debug info warn error fatal panic disabled"`
}

// ConfigureLogger configures the global zerolog logger based on environment variables.
// This should be called once at application startup.
func ConfigureLogger(ctx context.Context) {
	var err error

	var cfg ConfigLogging
	if err = envconfig.Process(ctx, &cfg); err != nil {
		golog.Fatalf("Cannot load logging configuration from environment. Reason: %v", err)
	}

	var level zerolog.Level
	if level, err = zerolog.ParseLevel(cfg.Level); err != nil {
		golog.Fatalf("Cannot parse logging level: %v", err)
	}
	zerolog.SetGlobalLevel(level)

	zerolog.CallerFieldName = cfg.EncodingFuncKey
	zerolog.ErrorFieldName = cfg.EncodingErrorKey
	zerolog.ErrorStackFieldName = cfg.EncodingStacktraceKey
	zerolog.LevelFieldName = cfg.EncodingLevelKey
	zerolog.MessageFieldName = cfg.EncodingMessageKey
	zerolog.TimestampFieldName = cfg.EncodingTimeKey

	var timeEncoders = map[ConfigLogTimeEncoder]string{
		ConfigLogTimeEncoderEpoch:       zerolog.TimeFormatUnix,
		ConfigLogTimeEncoderEpochmillis: zerolog.TimeFormatUnixMs,
		ConfigLogTimeEncoderEpochnanos:  zerolog.TimeFormatUnixNano,
		ConfigLogTimeEncoderIso8601:     "2006-01-02T15:04:05-0700",
		ConfigLogTimeEncoderRfc3339:     time.RFC3339,
		ConfigLogTimeEncoderRfc3339nano: time.RFC3339Nano,
	}
	if enc, ok := timeEncoders[cfg.EncodingTimeEncoder]; ok {
		zerolog.TimeFieldFormat = enc
	}

	if ConfigLogEncodingJson == cfg.Encoding {
		log.Logger = zerolog.New(os.Stdout).With().Timestamp().Caller().Logger()
	} else {
		log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: zerolog.TimeFieldFormat, NoColor: !cfg.EncodingColorize}).With().Timestamp().Caller().Logger()
	}
}
