package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	golog "log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"git.myservermanager.com/varakh/yafe/internal/auth"
	"git.myservermanager.com/varakh/yafe/internal/client"
	"git.myservermanager.com/varakh/yafe/internal/engine"
	"git.myservermanager.com/varakh/yafe/internal/frontend"
	"git.myservermanager.com/varakh/yafe/internal/output"
	"git.myservermanager.com/varakh/yafe/internal/queue"
	"git.myservermanager.com/varakh/yafe/internal/registry"
	"git.myservermanager.com/varakh/yafe/internal/scheduler"
	"git.myservermanager.com/varakh/yafe/internal/server"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

const (
	Name    = "YaFE"
	Version = "0.1.0"

	// Serve command flags
	flagQueueDir      = "queue-dir"
	flagFlowsDir      = "flows-dir"
	flagCleanupDone   = "cleanup-done"
	flagCleanupFailed = "cleanup-failed"
	flagPollInterval  = "poll-interval"
	flagSocket        = "socket"
	flagSocketFile    = "socket-file"
	flagHTTP          = "http"
	flagHTTPListen    = "http-listen"

	// Client connection flags (for jobs/flows commands)
	flagClientSocket = "socket"
	flagClientHTTP   = "http"
	flagAPIKey       = "api-key"

	// Command-specific flags
	flagInput  = "input"
	flagStatus = "status"
	flagRaw    = "raw"

	// Auth flags (serve command)
	flagAuthUser   = "auth-user"
	flagAuthKey    = "auth-key"
	flagAuthRole   = "auth-role"
	flagAuthFile   = "auth-file"
	flagSocketAuth = "auth-socket"
	flagHTTPAuth   = "auth-http"

	// Auth hash command flags
	flagKey = "key"

	// Schedule command flags
	flagFlow     = "flow"
	flagCron     = "cron"
	flagInterval = "interval"
	flagDisabled = "disabled"

	// Environment variables (client)
	envSocket = "YAFE_SOCKET"
	envHTTP   = "YAFE_HTTP"
	envAPIKey = "YAFE_API_KEY"

	// Environment variables (serve)
	envSocketEnabled = "YAFE_SOCKET_ENABLED"
	envSocketFile    = "YAFE_SOCKET_FILE"
	envHTTPEnabled   = "YAFE_HTTP_ENABLED"
	envHTTPListen    = "YAFE_HTTP_LISTEN"
	envQueueDir      = "YAFE_QUEUE_DIR"
	envFlowsDir      = "YAFE_FLOWS_DIR"
	envSchedulesDir  = "YAFE_SCHEDULES_DIR"
	envCleanupDone   = "YAFE_CLEANUP_DONE"
	envCleanupFailed = "YAFE_CLEANUP_FAILED"
	envPollInterval  = "YAFE_POLL_INTERVAL"
	envAuthUser      = "YAFE_AUTH_USER"
	envAuthKey       = "YAFE_AUTH_KEY"
	envAuthRole      = "YAFE_AUTH_ROLE"
	envAuthFile      = "YAFE_AUTH_FILE"
	envSocketAuth    = "YAFE_AUTH_SOCKET"
	envHTTPAuth      = "YAFE_AUTH_HTTP"

	// Environment variables (system)
	envXDGRuntime   = "XDG_RUNTIME_DIR"
	envXDGStateHome = "XDG_STATE_HOME"
)

func main() {
	application := &cli.Command{
		Name:    Name,
		Usage:   "command-line interface for YaFE",
		Version: Version,
		Commands: []*cli.Command{
			runCmd,
			serveCmd,
			jobsCmd,
			flowsCmd,
			schedulesCmd,
			authCmd,
		},
	}

	if err := application.Run(context.Background(), os.Args); err != nil {
		golog.Fatal(err)
	}
}

// clientFlags are shared by jobs and flows commands for daemon communication.
var clientFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    flagClientSocket,
		Aliases: []string{"S"},
		Usage:   "Unix socket path for daemon communication",
		Sources: cli.EnvVars(envSocket),
	},
	&cli.StringFlag{
		Name:    flagClientHTTP,
		Aliases: []string{"H"},
		Usage:   "HTTP URL for daemon communication (e.g., http://localhost:8080)",
		Sources: cli.EnvVars(envHTTP),
	},
	&cli.StringFlag{
		Name:    flagAPIKey,
		Usage:   "API key for authentication",
		Sources: cli.EnvVars(envAPIKey),
	},
}

var jobsCmd = &cli.Command{
	Name:  "jobs",
	Usage: "manage jobs in the queue",
	Flags: clientFlags,
	Commands: []*cli.Command{
		jobsListCmd,
		jobsEnqueueCmd,
		jobsGetCmd,
		jobsLogsCmd,
		jobsDequeueCmd,
	},
}

var flowsCmd = &cli.Command{
	Name:  "flows",
	Usage: "manage flow definitions",
	Flags: clientFlags,
	Commands: []*cli.Command{
		flowsListCmd,
		flowsGetCmd,
		flowsAddCmd,
		flowsRenameCmd,
		flowsRemoveCmd,
	},
}

var schedulesCmd = &cli.Command{
	Name:  "schedules",
	Usage: "manage schedules",
	Flags: clientFlags,
	Commands: []*cli.Command{
		schedulesListCmd,
		schedulesGetCmd,
		schedulesAddCmd,
		schedulesUpdateCmd,
		schedulesRemoveCmd,
		schedulesEnableCmd,
		schedulesDisableCmd,
	},
}

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "run /path/to/flow.yaml directly",
	Action: func(ctx context.Context, cmd *cli.Command) error {
		if !cmd.Args().Present() || cmd.Args().Len() < 1 {
			return cli.Exit(errors.New("args required - try 'run help'"), 1)
		}

		engine.ConfigureLogger(ctx)
		e := engine.NewEngine()

		var err error
		var flow *engine.Flow
		if flow, err = e.LoadFromFile(cmd.Args().First()); err != nil {
			return cli.Exit(fmt.Errorf("run failed: %w", err), 1)
		}

		if err := e.Run(ctx, flow); err != nil {
			return cli.Exit(fmt.Errorf("run failed: %w", err), 1)
		}

		return nil
	},
}

var serveCmd = &cli.Command{
	Name:  "serve",
	Usage: "start the daemon",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    flagSocket,
			Aliases: []string{"S"},
			Value:   true,
			Usage:   "enable Unix socket listener",
			Sources: cli.EnvVars(envSocketEnabled),
		},
		&cli.StringFlag{
			Name:    flagSocketFile,
			Usage:   "Unix socket path (default: $XDG_RUNTIME_DIR/yafe/yafe.sock)",
			Sources: cli.EnvVars(envSocketFile),
		},
		&cli.BoolFlag{
			Name:    flagHTTP,
			Aliases: []string{"H"},
			Value:   true,
			Usage:   "enable HTTP listener",
			Sources: cli.EnvVars(envHTTPEnabled),
		},
		&cli.StringFlag{
			Name:    flagHTTPListen,
			Value:   ":8080",
			Usage:   "HTTP listen address (only used when --http is enabled)",
			Sources: cli.EnvVars(envHTTPListen),
		},
		&cli.StringFlag{
			Name:    flagQueueDir,
			Aliases: []string{"q"},
			Value:   "",
			Usage:   "queue directory (default: $XDG_STATE_HOME/yafe/queue)",
			Sources: cli.EnvVars(envQueueDir),
		},
		&cli.StringFlag{
			Name:    flagFlowsDir,
			Aliases: []string{"f"},
			Value:   "",
			Usage:   "flows directory (default: $XDG_STATE_HOME/yafe/flows)",
			Sources: cli.EnvVars(envFlowsDir),
		},
		&cli.StringFlag{
			Name:    "schedules-dir",
			Value:   "",
			Usage:   "schedules directory (default: $XDG_STATE_HOME/yafe/schedules)",
			Sources: cli.EnvVars(envSchedulesDir),
		},
		&cli.DurationFlag{
			Name:    flagCleanupDone,
			Value:   0,
			Usage:   "retention period for completed jobs (0 to disable cleanup)",
			Sources: cli.EnvVars(envCleanupDone),
		},
		&cli.DurationFlag{
			Name:    flagCleanupFailed,
			Value:   0,
			Usage:   "retention period for failed jobs (0 to disable cleanup)",
			Sources: cli.EnvVars(envCleanupFailed),
		},
		&cli.DurationFlag{
			Name:    flagPollInterval,
			Value:   queue.DefaultWorkerConfig().PollInterval,
			Usage:   "interval for polling the queue for new jobs",
			Sources: cli.EnvVars(envPollInterval),
		},
		// Auth flags
		&cli.StringFlag{
			Name:    flagAuthUser,
			Usage:   "single user name for auth",
			Sources: cli.EnvVars(envAuthUser),
		},
		&cli.StringFlag{
			Name:    flagAuthKey,
			Usage:   "single user bcrypt-hashed key",
			Sources: cli.EnvVars(envAuthKey),
		},
		&cli.StringFlag{
			Name:    flagAuthRole,
			Usage:   "single user roles (comma-separated)",
			Sources: cli.EnvVars(envAuthRole),
		},
		&cli.StringFlag{
			Name:    flagAuthFile,
			Usage:   "path to auth file (user:hash:roles format)",
			Sources: cli.EnvVars(envAuthFile),
		},
		&cli.BoolFlag{
			Name:    flagSocketAuth,
			Usage:   "require authentication for Unix socket",
			Sources: cli.EnvVars(envSocketAuth),
		},
		&cli.BoolFlag{
			Name:    flagHTTPAuth,
			Usage:   "require authentication for HTTP",
			Sources: cli.EnvVars(envHTTPAuth),
		},
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		socketEnabled := cmd.Bool(flagSocket)
		socketPath := cmd.String(flagSocketFile)
		httpEnabled := cmd.Bool(flagHTTP)
		httpAddr := cmd.String(flagHTTPListen)
		queueDir := cmd.String(flagQueueDir)
		flowsDir := cmd.String(flagFlowsDir)
		schedulesDir := cmd.String("schedules-dir")
		cleanupDone := cmd.Duration(flagCleanupDone)
		cleanupFailed := cmd.Duration(flagCleanupFailed)
		pollInterval := cmd.Duration(flagPollInterval)

		// Auth flags
		authUser := cmd.String(flagAuthUser)
		authKey := cmd.String(flagAuthKey)
		authRole := cmd.String(flagAuthRole)
		authFile := cmd.String(flagAuthFile)
		socketAuth := cmd.Bool(flagSocketAuth)
		httpAuth := cmd.Bool(flagHTTPAuth)

		// Validate at least one listener is enabled
		if !socketEnabled && !httpEnabled {
			return cli.Exit(errors.New("at least one listener must be enabled (--socket or --http)"), 1)
		}

		stateHome, err := resolveStateHome()
		if err != nil {
			return cli.Exit(err, 1)
		}

		if socketPath == "" {
			socketPath = defaultSocketPath()
		}
		if queueDir == "" {
			queueDir = filepath.Join(stateHome, "yafe", "queue")
		}
		if flowsDir == "" {
			flowsDir = filepath.Join(stateHome, "yafe", "flows")
		}
		if schedulesDir == "" {
			schedulesDir = filepath.Join(stateHome, "yafe", "schedules")
		}

		// Clear socket path if socket is disabled
		if !socketEnabled {
			socketPath = ""
		}
		// Clear HTTP address if HTTP is disabled
		if !httpEnabled {
			httpAddr = ""
		}

		// Build authenticator
		var authenticator auth.Authenticator
		if authFile != "" && authUser != "" {
			return cli.Exit(errors.New("cannot use both --auth-file and --auth-user"), 1)
		}
		if authFile != "" {
			fileAuth, err := auth.NewFileAuthenticator(authFile)
			if err != nil {
				return cli.Exit(fmt.Errorf("failed to load auth file: %w", err), 1)
			}
			authenticator = fileAuth
		} else if authUser != "" {
			if authKey == "" {
				return cli.Exit(errors.New("--auth-key required with --auth-user"), 1)
			}
			inlineAuth, err := auth.NewInlineAuthenticator(authUser, authKey, authRole)
			if err != nil {
				return cli.Exit(fmt.Errorf("failed to create auth: %w", err), 1)
			}
			authenticator = inlineAuth
		}

		// Warn if auth required but no authenticator configured
		if (socketAuth || httpAuth) && authenticator == nil {
			return cli.Exit(errors.New("auth required but no authenticator configured (use --auth-file or --auth-user)"), 1)
		}

		// Initialize registry
		reg, err := registry.NewFileFlowRegistry(flowsDir)
		if err != nil {
			return cli.Exit(fmt.Errorf("failed to initialize registry: %w", err), 1)
		}

		// Initialize queue with registry
		cleanupConfig := queue.CleanupConfig{
			DoneRetention:   cleanupDone,
			FailedRetention: cleanupFailed,
		}
		q, err := queue.NewFileQueue(queueDir, reg, cleanupConfig)
		if err != nil {
			return cli.Exit(fmt.Errorf("failed to initialize queue: %w", err), 1)
		}

		// Initialize scheduler
		schedStore, err := scheduler.NewFileStore(schedulesDir)
		if err != nil {
			return cli.Exit(fmt.Errorf("failed to initialize scheduler store: %w", err), 1)
		}
		sched := scheduler.New(schedStore, q, reg)

		// Configure logging and initialize engine
		engine.ConfigureLogger(ctx)
		e := engine.NewEngine()

		// Create worker
		workerConfig := queue.DefaultWorkerConfig()
		workerConfig.PollInterval = pollInterval
		w := queue.NewWorker(q, reg, e, workerConfig)

		// Create server with embedded frontend
		serverConfig := server.Config{
			SocketPath: socketPath,
			HTTPAddr:   httpAddr,
			SocketAuth: socketAuth,
			HTTPAuth:   httpAuth,
			Auth:       authenticator,
		}
		frontendFS, _ := fs.Sub(frontend.FrontendFS, "app/dist")
		s := server.New(q, reg, sched, serverConfig, frontendFS)

		// Handle shutdown
		ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		// Start scheduler
		if err := sched.Start(ctx); err != nil {
			return cli.Exit(fmt.Errorf("failed to start scheduler: %w", err), 1)
		}

		// Run worker in background
		go w.Run(ctx)

		// Run server (blocks until shutdown)
		if err := s.Run(ctx); err != nil {
			return cli.Exit(fmt.Errorf("server error: %w", err), 1)
		}

		// Stop scheduler
		sched.Stop()

		// Wait for worker to finish current job
		<-w.Done()

		return nil
	},
}

var jobsEnqueueCmd = &cli.Command{
	Name:      "enqueue",
	Usage:     "enqueue a job for a flow",
	ArgsUsage: "<flow-name>",
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:    flagInput,
			Aliases: []string{"i"},
			Usage:   "input key=value pairs (can be specified multiple times)",
		},
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		if !cmd.Args().Present() {
			return cli.Exit(errors.New("flow name required"), 1)
		}

		c, err := newClient(cmd)
		if err != nil {
			return cli.Exit(err, 1)
		}

		flowName := cmd.Args().First()
		inputPairs := cmd.StringSlice(flagInput)

		// Parse input key=value pairs
		inputs := make(map[string]string)
		for _, pair := range inputPairs {
			key, value, found := strings.Cut(pair, "=")
			if !found {
				return cli.Exit(fmt.Errorf("invalid input format %q, expected key=value", pair), 1)
			}
			inputs[key] = value
		}

		jobID, err := c.EnqueueJob(flowName, inputs)
		if err != nil {
			return cli.Exit(fmt.Errorf("failed to enqueue job: %w", err), 1)
		}

		fmt.Printf("Job enqueued: %s\n", jobID)
		return nil
	},
}

var jobsListCmd = &cli.Command{
	Name:  "list",
	Usage: "list jobs in the queue",
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:    flagStatus,
			Aliases: []string{"s"},
			Usage:   "filter by status (can be specified multiple times, default: all)",
		},
		&cli.BoolFlag{
			Name:  flagRaw,
			Usage: "output raw JSON",
		},
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		c, err := newClient(cmd)
		if err != nil {
			return cli.Exit(err, 1)
		}

		statuses := cmd.StringSlice(flagStatus)
		jobs, err := c.ListJobs(statuses)
		if err != nil {
			return cli.Exit(fmt.Errorf("failed to list jobs: %w", err), 1)
		}

		if cmd.Bool(flagRaw) {
			if len(jobs) == 0 {
				fmt.Println("[]")
				return nil
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(jobs); err != nil {
				return cli.Exit(fmt.Errorf("failed to encode jobs: %w", err), 1)
			}
			return nil
		}

		// Human-readable table output
		tbl := output.NewTable("ID", "STATUS", "FLOW", "CREATED", "STARTED", "ENDED", "ERROR")
		for _, job := range jobs {
			tbl.AddRow(
				job.ID,
				job.Status,
				job.Flow,
				output.FormatTimeValue(job.CreatedAt),
				output.FormatTime(job.StartedAt),
				output.FormatTime(job.EndedAt),
				output.TruncateString(output.FormatString(job.Error), 40),
			)
		}
		tbl.Render(os.Stdout)

		return nil
	},
}

var jobsGetCmd = &cli.Command{
	Name:      "get",
	Usage:     "get a job by ID",
	ArgsUsage: "<job-id>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  flagRaw,
			Usage: "output raw JSON",
		},
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		if !cmd.Args().Present() {
			return cli.Exit(errors.New("job ID required"), 1)
		}

		c, err := newClient(cmd)
		if err != nil {
			return cli.Exit(err, 1)
		}

		jobID := cmd.Args().First()
		job, err := c.GetJob(jobID)
		if err != nil {
			return cli.Exit(fmt.Errorf("failed to get job: %w", err), 1)
		}

		if cmd.Bool(flagRaw) {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(job); err != nil {
				return cli.Exit(fmt.Errorf("failed to encode job: %w", err), 1)
			}
			return nil
		}

		// Human-readable key-value output
		pairs := []output.KeyValue{
			{Key: "ID", Value: job.ID},
			{Key: "Flow", Value: job.Flow},
			{Key: "Status", Value: job.Status},
			{Key: "Created", Value: output.FormatTimeValue(job.CreatedAt)},
			{Key: "Started", Value: output.FormatTime(job.StartedAt)},
			{Key: "Ended", Value: output.FormatTime(job.EndedAt)},
			{Key: "Exit Code", Value: output.FormatInt(job.ExitCode)},
			{Key: "Error", Value: output.FormatString(job.Error)},
		}
		output.PrintKeyValuesWithIndent(os.Stdout, pairs, "")

		// Print inputs if any
		if len(job.Inputs) > 0 {
			fmt.Println("Inputs:")
			// Sort keys for consistent output
			keys := make([]string, 0, len(job.Inputs))
			for k := range job.Inputs {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Printf("  %s: %s\n", k, job.Inputs[k])
			}
		}

		return nil
	},
}

var jobsLogsCmd = &cli.Command{
	Name:      "logs",
	Usage:     "get logs for a completed job",
	ArgsUsage: "<job-id>",
	Action: func(ctx context.Context, cmd *cli.Command) error {
		if !cmd.Args().Present() {
			return cli.Exit(errors.New("job ID required"), 1)
		}

		c, err := newClient(cmd)
		if err != nil {
			return cli.Exit(err, 1)
		}

		jobID := cmd.Args().First()
		logs, err := c.GetJobLogs(jobID)
		if err != nil {
			return cli.Exit(fmt.Errorf("failed to get job logs: %w", err), 1)
		}

		fmt.Print(logs)
		return nil
	},
}

var jobsDequeueCmd = &cli.Command{
	Name:      "dequeue",
	Usage:     "delete a job by ID",
	ArgsUsage: "<job-id>",
	Action: func(ctx context.Context, cmd *cli.Command) error {
		if !cmd.Args().Present() {
			return cli.Exit(errors.New("job ID required"), 1)
		}

		c, err := newClient(cmd)
		if err != nil {
			return cli.Exit(err, 1)
		}

		jobID := cmd.Args().First()
		if err := c.DeleteJob(jobID); err != nil {
			return cli.Exit(fmt.Errorf("failed to delete job: %w", err), 1)
		}

		fmt.Printf("Job deleted: %s\n", jobID)
		return nil
	},
}

var flowsListCmd = &cli.Command{
	Name:  "list",
	Usage: "list all flows",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  flagRaw,
			Usage: "output raw JSON",
		},
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		c, err := newClient(cmd)
		if err != nil {
			return cli.Exit(err, 1)
		}

		flows, err := c.ListFlows()
		if err != nil {
			return cli.Exit(fmt.Errorf("failed to list flows: %w", err), 1)
		}

		if cmd.Bool(flagRaw) {
			if len(flows) == 0 {
				fmt.Println("[]")
				return nil
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(flows); err != nil {
				return cli.Exit(fmt.Errorf("failed to encode flows: %w", err), 1)
			}
			return nil
		}

		// Human-readable table output
		tbl := output.NewTable("NAME")
		for _, flow := range flows {
			tbl.AddRow(flow)
		}
		tbl.Render(os.Stdout)

		return nil
	},
}

var flowsGetCmd = &cli.Command{
	Name:      "get",
	Usage:     "get a flow by name",
	ArgsUsage: "<flow-name>",
	Action: func(ctx context.Context, cmd *cli.Command) error {
		if !cmd.Args().Present() {
			return cli.Exit(errors.New("flow name required"), 1)
		}

		c, err := newClient(cmd)
		if err != nil {
			return cli.Exit(err, 1)
		}

		flowName := cmd.Args().First()
		flow, err := c.GetFlow(flowName)
		if err != nil {
			return cli.Exit(fmt.Errorf("failed to get flow: %w", err), 1)
		}

		fmt.Print(flow.Content)
		return nil
	},
}

var flowsAddCmd = &cli.Command{
	Name:      "add",
	Usage:     "add a flow from a file",
	ArgsUsage: "<flow-name> <flow-file>",
	Action: func(ctx context.Context, cmd *cli.Command) error {
		if cmd.Args().Len() < 2 {
			return cli.Exit(errors.New("flow name and flow file required"), 1)
		}

		c, err := newClient(cmd)
		if err != nil {
			return cli.Exit(err, 1)
		}

		flowName := cmd.Args().Get(0)
		flowFile := cmd.Args().Get(1)

		// Read flow file content
		//gosec:disable G304 -- Explicitly allowed to use any location
		content, err := os.ReadFile(flowFile)
		if err != nil {
			return cli.Exit(fmt.Errorf("failed to read flow file: %w", err), 1)
		}

		if err := c.AddFlow(flowName, content); err != nil {
			return cli.Exit(fmt.Errorf("failed to add flow: %w", err), 1)
		}

		fmt.Printf("Flow added: %s\n", flowName)
		return nil
	},
}

var flowsRenameCmd = &cli.Command{
	Name:      "rename",
	Aliases:   []string{"mv"},
	Usage:     "rename a flow",
	ArgsUsage: "<old-name> <new-name>",
	Action: func(ctx context.Context, cmd *cli.Command) error {
		if cmd.Args().Len() < 2 {
			return cli.Exit(errors.New("old name and new name required"), 1)
		}

		c, err := newClient(cmd)
		if err != nil {
			return cli.Exit(err, 1)
		}

		oldName := cmd.Args().Get(0)
		newName := cmd.Args().Get(1)

		if err := c.RenameFlow(oldName, newName); err != nil {
			return cli.Exit(fmt.Errorf("failed to rename flow: %w", err), 1)
		}

		fmt.Printf("Flow renamed: %s -> %s\n", oldName, newName)
		return nil
	},
}

var flowsRemoveCmd = &cli.Command{
	Name:      "rm",
	Usage:     "remove a flow by name",
	ArgsUsage: "<flow-name>",
	Action: func(ctx context.Context, cmd *cli.Command) error {
		if !cmd.Args().Present() {
			return cli.Exit(errors.New("flow name required"), 1)
		}

		c, err := newClient(cmd)
		if err != nil {
			return cli.Exit(err, 1)
		}

		flowName := cmd.Args().First()
		if err := c.DeleteFlow(flowName); err != nil {
			return cli.Exit(fmt.Errorf("failed to delete flow: %w", err), 1)
		}

		fmt.Printf("Flow deleted: %s\n", flowName)
		return nil
	},
}

var schedulesListCmd = &cli.Command{
	Name:  "list",
	Usage: "list all schedules",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  flagRaw,
			Usage: "output raw JSON",
		},
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		c, err := newClient(cmd)
		if err != nil {
			return cli.Exit(err, 1)
		}

		schedules, err := c.ListSchedules()
		if err != nil {
			return cli.Exit(fmt.Errorf("failed to list schedules: %w", err), 1)
		}

		if cmd.Bool(flagRaw) {
			if len(schedules) == 0 {
				fmt.Println("[]")
				return nil
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(schedules); err != nil {
				return cli.Exit(fmt.Errorf("failed to encode schedules: %w", err), 1)
			}
			return nil
		}

		// Human-readable table output
		tbl := output.NewTable("NAME", "FLOW", "TYPE", "EXPRESSION", "ENABLED", "NEXT RUN")
		for _, sched := range schedules {
			enabled := "no"
			if sched.Enabled {
				enabled = "yes"
			}
			nextRun := "-"
			if sched.NextRunAt != nil {
				nextRun = output.FormatTimeValue(*sched.NextRunAt)
			}
			tbl.AddRow(
				sched.Name,
				sched.Flow,
				sched.Type,
				sched.Expression,
				enabled,
				nextRun,
			)
		}
		tbl.Render(os.Stdout)

		return nil
	},
}

var schedulesGetCmd = &cli.Command{
	Name:      "get",
	Usage:     "get a schedule by name",
	ArgsUsage: "<schedule-name>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  flagRaw,
			Usage: "output raw JSON",
		},
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		if !cmd.Args().Present() {
			return cli.Exit(errors.New("schedule name required"), 1)
		}

		c, err := newClient(cmd)
		if err != nil {
			return cli.Exit(err, 1)
		}

		scheduleName := cmd.Args().First()
		sched, err := c.GetSchedule(scheduleName)
		if err != nil {
			return cli.Exit(fmt.Errorf("failed to get schedule: %w", err), 1)
		}

		if cmd.Bool(flagRaw) {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(sched); err != nil {
				return cli.Exit(fmt.Errorf("failed to encode schedule: %w", err), 1)
			}
			return nil
		}

		// Human-readable key-value output
		enabled := "no"
		if sched.Enabled {
			enabled = "yes"
		}
		pairs := []output.KeyValue{
			{Key: "Name", Value: sched.Name},
			{Key: "Flow", Value: sched.Flow},
			{Key: "Type", Value: sched.Type},
			{Key: "Expression", Value: sched.Expression},
			{Key: "Enabled", Value: enabled},
			{Key: "Created", Value: output.FormatTimeValue(sched.CreatedAt)},
			{Key: "Last Run", Value: output.FormatTime(sched.LastRunAt)},
			{Key: "Next Run", Value: output.FormatTime(sched.NextRunAt)},
		}
		output.PrintKeyValuesWithIndent(os.Stdout, pairs, "")

		// Print inputs if any
		if len(sched.Inputs) > 0 {
			fmt.Println("Inputs:")
			keys := make([]string, 0, len(sched.Inputs))
			for k := range sched.Inputs {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Printf("  %s: %s\n", k, sched.Inputs[k])
			}
		}

		return nil
	},
}

var schedulesAddCmd = &cli.Command{
	Name:      "add",
	Usage:     "add a new schedule",
	ArgsUsage: "<schedule-name>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     flagFlow,
			Aliases:  []string{"f"},
			Usage:    "flow name to trigger",
			Required: true,
		},
		&cli.StringFlag{
			Name:    flagCron,
			Aliases: []string{"c"},
			Usage:   "cron expression (6 fields with seconds, e.g., '0 0 2 * * *')",
		},
		&cli.StringFlag{
			Name:    flagInterval,
			Aliases: []string{"I"},
			Usage:   "interval duration (e.g., '5m', '1h')",
		},
		&cli.StringSliceFlag{
			Name:    flagInput,
			Aliases: []string{"i"},
			Usage:   "input key=value pairs (can be specified multiple times)",
		},
		&cli.BoolFlag{
			Name:  flagDisabled,
			Usage: "create schedule in disabled state",
		},
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		if !cmd.Args().Present() {
			return cli.Exit(errors.New("schedule name required"), 1)
		}

		cronExpr := cmd.String(flagCron)
		intervalExpr := cmd.String(flagInterval)

		if cronExpr == "" && intervalExpr == "" {
			return cli.Exit(errors.New("either --cron or --interval is required"), 1)
		}
		if cronExpr != "" && intervalExpr != "" {
			return cli.Exit(errors.New("--cron and --interval are mutually exclusive"), 1)
		}

		c, err := newClient(cmd)
		if err != nil {
			return cli.Exit(err, 1)
		}

		scheduleName := cmd.Args().First()
		inputPairs := cmd.StringSlice(flagInput)

		// Parse input key=value pairs
		inputs := make(map[string]string)
		for _, pair := range inputPairs {
			key, value, found := strings.Cut(pair, "=")
			if !found {
				return cli.Exit(fmt.Errorf("invalid input format %q, expected key=value", pair), 1)
			}
			inputs[key] = value
		}

		var schedType, expression string
		if cronExpr != "" {
			schedType = "cron"
			expression = cronExpr
		} else {
			schedType = "interval"
			expression = intervalExpr
		}

		req := client.CreateScheduleRequest{
			Name:       scheduleName,
			Flow:       cmd.String(flagFlow),
			Type:       schedType,
			Expression: expression,
			Inputs:     inputs,
			Enabled:    !cmd.Bool(flagDisabled),
		}

		if err := c.CreateSchedule(req); err != nil {
			return cli.Exit(fmt.Errorf("failed to create schedule: %w", err), 1)
		}

		fmt.Printf("Schedule created: %s\n", scheduleName)
		return nil
	},
}

var schedulesUpdateCmd = &cli.Command{
	Name:      "update",
	Usage:     "update an existing schedule",
	ArgsUsage: "<schedule-name>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    flagFlow,
			Aliases: []string{"f"},
			Usage:   "flow name to trigger",
		},
		&cli.StringFlag{
			Name:    flagCron,
			Aliases: []string{"c"},
			Usage:   "cron expression (6 fields with seconds)",
		},
		&cli.StringFlag{
			Name:    flagInterval,
			Aliases: []string{"I"},
			Usage:   "interval duration (e.g., '5m', '1h')",
		},
		&cli.StringSliceFlag{
			Name:    flagInput,
			Aliases: []string{"i"},
			Usage:   "input key=value pairs (replaces all inputs)",
		},
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		if !cmd.Args().Present() {
			return cli.Exit(errors.New("schedule name required"), 1)
		}

		cronExpr := cmd.String(flagCron)
		intervalExpr := cmd.String(flagInterval)

		if cronExpr != "" && intervalExpr != "" {
			return cli.Exit(errors.New("--cron and --interval are mutually exclusive"), 1)
		}

		c, err := newClient(cmd)
		if err != nil {
			return cli.Exit(err, 1)
		}

		scheduleName := cmd.Args().First()

		// Get existing schedule to merge with updates
		existing, err := c.GetSchedule(scheduleName)
		if err != nil {
			return cli.Exit(fmt.Errorf("failed to get schedule: %w", err), 1)
		}

		// Apply updates
		flow := existing.Flow
		if cmd.IsSet(flagFlow) {
			flow = cmd.String(flagFlow)
		}

		schedType := existing.Type
		expression := existing.Expression
		if cronExpr != "" {
			schedType = "cron"
			expression = cronExpr
		} else if intervalExpr != "" {
			schedType = "interval"
			expression = intervalExpr
		}

		inputs := existing.Inputs
		if cmd.IsSet(flagInput) {
			inputPairs := cmd.StringSlice(flagInput)
			inputs = make(map[string]string)
			for _, pair := range inputPairs {
				key, value, found := strings.Cut(pair, "=")
				if !found {
					return cli.Exit(fmt.Errorf("invalid input format %q, expected key=value", pair), 1)
				}
				inputs[key] = value
			}
		}

		req := client.UpdateScheduleRequest{
			Flow:       flow,
			Type:       schedType,
			Expression: expression,
			Inputs:     inputs,
			Enabled:    existing.Enabled,
		}

		if err := c.UpdateSchedule(scheduleName, req); err != nil {
			return cli.Exit(fmt.Errorf("failed to update schedule: %w", err), 1)
		}

		fmt.Printf("Schedule updated: %s\n", scheduleName)
		return nil
	},
}

var schedulesRemoveCmd = &cli.Command{
	Name:      "rm",
	Usage:     "remove a schedule by name",
	ArgsUsage: "<schedule-name>",
	Action: func(ctx context.Context, cmd *cli.Command) error {
		if !cmd.Args().Present() {
			return cli.Exit(errors.New("schedule name required"), 1)
		}

		c, err := newClient(cmd)
		if err != nil {
			return cli.Exit(err, 1)
		}

		scheduleName := cmd.Args().First()
		if err := c.DeleteSchedule(scheduleName); err != nil {
			return cli.Exit(fmt.Errorf("failed to delete schedule: %w", err), 1)
		}

		fmt.Printf("Schedule deleted: %s\n", scheduleName)
		return nil
	},
}

var schedulesEnableCmd = &cli.Command{
	Name:      "enable",
	Usage:     "enable a schedule",
	ArgsUsage: "<schedule-name>",
	Action: func(ctx context.Context, cmd *cli.Command) error {
		if !cmd.Args().Present() {
			return cli.Exit(errors.New("schedule name required"), 1)
		}

		c, err := newClient(cmd)
		if err != nil {
			return cli.Exit(err, 1)
		}

		scheduleName := cmd.Args().First()
		if err := c.EnableSchedule(scheduleName); err != nil {
			return cli.Exit(fmt.Errorf("failed to enable schedule: %w", err), 1)
		}

		fmt.Printf("Schedule enabled: %s\n", scheduleName)
		return nil
	},
}

var schedulesDisableCmd = &cli.Command{
	Name:      "disable",
	Usage:     "disable a schedule",
	ArgsUsage: "<schedule-name>",
	Action: func(ctx context.Context, cmd *cli.Command) error {
		if !cmd.Args().Present() {
			return cli.Exit(errors.New("schedule name required"), 1)
		}

		c, err := newClient(cmd)
		if err != nil {
			return cli.Exit(err, 1)
		}

		scheduleName := cmd.Args().First()
		if err := c.DisableSchedule(scheduleName); err != nil {
			return cli.Exit(fmt.Errorf("failed to disable schedule: %w", err), 1)
		}

		fmt.Printf("Schedule disabled: %s\n", scheduleName)
		return nil
	},
}

func newClient(cmd *cli.Command) (*client.Client, error) {
	apiKey := cmd.String(flagAPIKey)

	// Check for HTTP flag first (takes precedence)
	if httpURL := cmd.String(flagClientHTTP); httpURL != "" {
		return client.NewHTTPClient(httpURL, apiKey), nil
	}

	// Use Unix socket
	socketPath := cmd.String(flagClientSocket)
	if socketPath == "" {
		socketPath = defaultSocketPath()
	}

	// Check socket exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("daemon not running (socket not found: %s)\nStart the daemon with: yafe serve", socketPath)
	}

	return client.NewUnixSocketClient(socketPath, apiKey), nil
}

func defaultSocketPath() string {
	runtimeDir := os.Getenv(envXDGRuntime)
	if runtimeDir == "" {
		// Fallback for systems without XDG_RUNTIME_DIR
		runtimeDir = fmt.Sprintf("/tmp/yafe-%d", os.Getuid())
	}
	return filepath.Join(runtimeDir, "yafe", "yafe.sock")
}

func resolveStateHome() (string, error) {
	stateHome := os.Getenv(envXDGStateHome)
	if stateHome == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		stateHome = filepath.Join(homeDir, ".local", "state")
	}
	return stateHome, nil
}

var authCmd = &cli.Command{
	Name:  "auth",
	Usage: "authentication utilities",
	Commands: []*cli.Command{
		authHashCmd,
	},
}

var authHashCmd = &cli.Command{
	Name:  "hash",
	Usage: "generate bcrypt hash for an API key",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  flagKey,
			Usage: "API key to hash (insecure, prefer interactive prompt)",
		},
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		key := cmd.String(flagKey)

		if key == "" {
			// Interactive prompt
			fmt.Print("Enter API key: ")
			keyBytes, err := term.ReadPassword(syscall.Stdin)
			fmt.Println()
			if err != nil {
				// Fallback for non-terminal
				reader := bufio.NewReader(os.Stdin)
				line, err := reader.ReadString('\n')
				if err != nil {
					return cli.Exit(fmt.Errorf("failed to read key: %w", err), 1)
				}
				key = strings.TrimSpace(line)
			} else {
				key = string(keyBytes)
			}
		}

		if key == "" {
			return cli.Exit(errors.New("API key cannot be empty"), 1)
		}

		hash, err := auth.HashKey(key)
		if err != nil {
			return cli.Exit(fmt.Errorf("failed to hash key: %w", err), 1)
		}

		fmt.Println(string(hash))
		return nil
	},
}
