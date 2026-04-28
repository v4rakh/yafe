package engine

// Environment variables injected into steps
const (
	EnvYafeState  = "YAFE_STATE"
	EnvYafeOutput = "YAFE_OUTPUT"
)

// State directory constants
const (
	stateDirPrefix = "yafe-"
	outputsDirName = "_outputs"
	toolsDirName   = "_tools"
)

// Tool constants
const (
	toolMetaSuffix = ".meta.json"
	defaultRetries = 1
)

// Shell step constants
const (
	defaultShell   = "bash"
	shellShebang   = "/usr/bin/env"
	scriptFileMode = 0755
	dirMode        = 0755
	fileMode       = 0644
)

// Script file naming
const (
	scriptPrefix = "flow-%d-%s-"
	scriptSuffix = "*.sh"
)

// Secret masking
const (
	secretMask = "***"
)
