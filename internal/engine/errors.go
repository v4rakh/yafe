package engine

import "errors"

// Engine errors
var (
	ErrLoadRead         = errors.New("reading failed")
	ErrLoadParsing      = errors.New("parsing failed")
	ErrRunsOnBootstrap  = errors.New("bootstrapping failed")
	ErrRunsOnTeardown   = errors.New("teardown failed")
	ErrStepIncompatible = errors.New("incompatible step for runs-on")
	ErrStepExecution    = errors.New("step execution failed")
	ErrValidationNotNil = errors.New("assert: nil values are not allowed")
)

// State sharing errors
var (
	ErrDuplicateStepName        = errors.New("duplicate step name")
	ErrMissingOutputPath        = errors.New("file output requires path")
	ErrTemplateInvalidReference = errors.New("invalid template reference")
	ErrOutputNotProduced        = errors.New("declared output not produced")
	ErrStateInitialize          = errors.New("state initialization failed")
	ErrPathTraversal            = errors.New("path traversal attempt")
)

// Shell step errors
var (
	ErrShellStepTempScriptCreation   = errors.New("creating temporary shell step script failed")
	ErrShellStepTempScriptExecutable = errors.New("setting executable flag on temporary shell step script failed")
	ErrShellStepTempScriptIO         = errors.New("writing temporary shell step script failed")
	ErrShellStepExecution            = errors.New("executing temporary shell step script failed")
)

// Secret errors
var (
	ErrSecretNotFound    = errors.New("secret not found")
	ErrSecretSourceEmpty = errors.New("secret source not specified")
	ErrSecretEnvNotSet   = errors.New("secret environment variable not set")
	ErrSecretFileRead    = errors.New("secret file read failed")
	ErrDuplicateSecret   = errors.New("duplicate secret name")
)

// Tool errors
var (
	ErrToolNotFound       = errors.New("tool not found in PATH")
	ErrToolDownload       = errors.New("tool download failed")
	ErrToolChecksum       = errors.New("tool checksum mismatch")
	ErrToolExtract        = errors.New("tool archive extraction failed")
	ErrToolPathRequired   = errors.New("path required for archive")
	ErrToolPathForbidden  = errors.New("path forbidden for non-archive")
	ErrDuplicateTool      = errors.New("duplicate tool name")
	ErrToolNameRequired   = errors.New("tool name is required")
	ErrToolInvalidRetries = errors.New("retries must be >= 1")
	ErrToolBinaryNotFound = errors.New("binary not found in archive")
)
