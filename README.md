# README

YaFE (**Y**et **a**nother **F**low **E**ngine) - a lightweight **Go-based workflow engine** that executes steps defined
in simple YAML files. Ideal for small CI/CD, deployment scripts, and other automation tasks.

It's a single binary and features a full web interface.

## Install

You can either **download the binary** for your operating system, use the provided **docker image** (see [container usage](#container)), or add yafe as **Nix flakes** input.

```nix
# flake.nix
{
    inputs.yafe.url = "git+https://git.myservermanager.com/varakh/yafe?ref=refs/tags/latest";
}
```

## Usage

### Direct Execution

Create your flow `myflow.yaml`

```yaml
runs-on: host
steps:
    -   cmd: echo "Flow execution works!"
        kind: shell
```

Then, run it with `yafe run myflow.yaml`.

### Daemon Mode

Start the daemon to manage flows and jobs via CLI or REST API:

```shell
yafe serve
```

The daemon listens on Unix socket (`$XDG_RUNTIME_DIR/yafe/yafe.sock`) and HTTP (`:8080`) by default.

**Available configuration for daemon mode:**.

All of these can also be specified with direct command-line arguments, e.g., `yafe serve --queue-dir=...`.

| Variable              | Description                                | Default           |
|-----------------------|--------------------------------------------|-------------------|
| `YAFE_SOCKET_ENABLED` | Enable Unix socket listener                | `false`           |
| `YAFE_SOCKET_FILE`    | Unix socket path                           | -                 |
| `YAFE_HTTP_ENABLED`   | Enable HTTP listener                       | `true`            |
| `YAFE_HTTP_LISTEN`    | HTTP listen address                        | `:8080`           |
| `YAFE_QUEUE_DIR`      | Queue directory                            | `/data/queue`     |
| `YAFE_FLOWS_DIR`      | Flows directory                            | `/data/flows`     |
| `YAFE_CLEANUP_DONE`   | Retention for completed jobs (e.g., `24h`) | `0` (disabled)    |
| `YAFE_CLEANUP_FAILED` | Retention for failed jobs (e.g., `168h`)   | `0` (disabled)    |
| `YAFE_POLL_INTERVAL`  | Queue poll interval                        | `1s`              |
| `YAFE_SCHEDULES_DIR`  | Schedules directory                        | `/data/schedules` |
| `YAFE_AUTH_USER`      | Single user name                           | -                 |
| `YAFE_AUTH_KEY`       | Single user bcrypt-hashed key              | -                 |
| `YAFE_AUTH_ROLE`      | Single user roles (comma-separated)        | -                 |
| `YAFE_AUTH_FILE`      | Path to auth file                          | -                 |
| `YAFE_AUTH_SOCKET`    | Require auth for Unix socket               | `false`           |
| `YAFE_AUTH_HTTP`      | Require auth for HTTP                      | `false`           |

Clients run against a started daemon. These can be specified with respective command-line options or with environment
variables for client calls

| Variable       | Command-line argument | Description                                                   |
|----------------|-----------------------|---------------------------------------------------------------|
| `YAFE_SOCKET`  | `--socket` or `-S`    | Unix socket path for daemon communication                     |
| `YAFE_HTTP`    | `--http` or `-H`      | HTTP URL for daemon communication                             |
| `YAFE_API_KEY` | `--api-key`           | API key for authentication (only required when daemon uses it |

These will be omitted in the following examples. Remember to provide them to point the client to a yafe instance.

**Managing flows:**

```shell
yafe flows add my-flow path/to/flow.yaml    # Register a flow
yafe flows list                             # List registered flows
yafe flows get my-flow                      # Show flow content
yafe flows rm my-flow                       # Remove a flow
```

**Managing jobs:**

```shell
yafe jobs enqueue my-flow -i key=value   # Queue a job with inputs
yafe jobs list                           # List all jobs
yafe jobs list -s pending -s running     # Filter by status
yafe jobs get <job-id>                   # Show job details
yafe jobs dequeue <job-id>               # Delete a job
```

**Managing schedules:**

```shell
yafe schedules add my-schedule my-flow --type cron --expression "0 * * * *"  # Create hourly schedule
yafe schedules add my-schedule my-flow --type interval --expression 5m       # Create interval schedule
yafe schedules list                      # List all schedules
yafe schedules get my-schedule           # Show schedule details
yafe schedules update my-schedule --expression "0 0 * * *"                   # Update schedule
yafe schedules enable my-schedule        # Enable a schedule
yafe schedules disable my-schedule       # Disable a schedule
yafe schedules rm my-schedule            # Delete a schedule
```

**Output format:**

```shell
yafe jobs list --raw    # JSON output (default is human-readable table)
```

### Job Inputs

When enqueueing a job, inputs are passed via `-i key=value` flags (CLI) or the `inputs` field (REST API). Inside flows,
these are available as `YAFE_INPUT_<KEY>` environment variables (uppercase):

```shell
yafe jobs enqueue deploy -i environment=prod -i version=1.2.3
```

```yaml
# In your flow deploy.yaml
steps:
    -   cmd: echo "Deploying $YAFE_INPUT_VERSION to $YAFE_INPUT_ENVIRONMENT"
        kind: shell
```

### Authentication

The daemon API can be secured with API key authentication and role-based access control.

**Generating a hashed key:**

```shell
yafe auth hash
# Enter API key: ****
# $2a$10$...
```

**Auth file format** (`user:bcrypt_hash:roles`):

```
# Comments start with #
admin:$2a$10$...:jobs:read,jobs:write,flows:read,flows:write,schedules:read,schedules:write
viewer:$2a$10$...:jobs:read,flows:read,schedules:read
deployer:$2a$10$...:jobs:write,flows:read
```

**Available roles:**

| Role              | Permissions                                      |
|-------------------|--------------------------------------------------|
| `jobs:read`       | List jobs, get a job's details                   |
| `jobs:write`      | Enqueue and dequeue jobs                         |
| `flows:read`      | List flows, get a flow's details                 |
| `flows:write`     | Add (or overwrite) a flow and delete a flow      |
| `schedules:read`  | List schedules, get a schedule's details         |
| `schedules:write` | Create, update, delete, enable/disable schedules |

**Starting the daemon with authentication:**

```shell
# Using auth file (recommended)
yafe serve --auth-file /etc/yafe/auth --http-auth

# Using inline credentials (single user)
yafe serve --auth-user admin --auth-key '$2a$10$...' --auth-role 'jobs:read,jobs:write,schedules:read' --http-auth

# Require auth for both HTTP and Unix socket
yafe serve --auth-file /etc/yafe/auth --http-auth --socket-auth
```

**Client authentication:**

```shell
# Via environment variable (recommended)
export YAFE_API_KEY=mysecretkey
yafe jobs list

# Via flag
yafe --api-key mysecretkey jobs list
```

### Container

Run with default settings (HTTP on port 8080, no auth):

```shell
docker run -d -p 8080:8080 -v yafe-data:/data git.myservermanager.com/varakh/yafe
```

**Docker Compose with authentication:**

```yaml
services:
    yafe:
        image: yafe
        ports:
            - "8080:8080"
        volumes:
            - yafe-data:/data
        environment:
            - YAFE_AUTH_HTTP=true
            - YAFE_AUTH_USER=admin
            - YAFE_AUTH_KEY=$$2a$$10$$... # bcrypt hash (escape $ with $$)
            - YAFE_AUTH_ROLE=jobs:read,jobs:write,flows:read,flows:write,schedules:read,schedules:write
            - YAFE_CLEANUP_DONE=24h
            - YAFE_CLEANUP_FAILED=168h

volumes:
    yafe-data:
```

**Using an auth file:**

```yaml
services:
    yafe:
        image: yafe
        ports:
            - "8080:8080"
        volumes:
            - yafe-data:/data
            - ./auth:/etc/yafe/auth:ro
        environment:
            - YAFE_AUTH_HTTP=true
            - YAFE_AUTH_FILE=/etc/yafe/auth

volumes:
    yafe-data:
```

### REST API

See [OpenAPI specification](_doc/openapi.yaml) for the full API specification.

## YAML Schema Reference

This outlines the full specification of a flow and which keywords you can use in your YAML file.

Additional examples are provided below or inside the `_doc/` folder.

### Top-Level Fields

| Field       | Type                       | Required | Default             | Description                               |
|-------------|----------------------------|----------|---------------------|-------------------------------------------|
| `runs-on`   | `string`                   | ✅ Yes    | -                   | Execution environment. Valid: `host`      |
| `steps`     | `array[Step]`              | ✅ Yes    | -                   | Ordered list of executable steps          |
| `state-dir` | `string`                   | No       | `/tmp/yafe-<runID>` | Directory for state and output files      |
| `secrets`   | `array[SecretDeclaration]` | No       | `[]`                | Flow-level secrets available to all steps |

### Step Fields

| Field     | Type                       | Required | Default | Description                                           |
|-----------|----------------------------|----------|---------|-------------------------------------------------------|
| `kind`    | `string`                   | ✅ Yes    | -       | Step type. Valid: `shell`                             |
| `cmd`     | `string`                   | ✅ Yes    | -       | Single or multi-line command to execute               |
| `name`    | `string`                   | No*      | -       | Unique step identifier. *Required if using `outputs`  |
| `shell`   | `string`                   | No       | `bash`  | Shell interpreter. Valid: `bash`, `sh`, `zsh`         |
| `env`     | `array[string]`            | No       | `[]`    | Environment variables in `KEY=value` format           |
| `outputs` | `array[StepOutput]`        | No       | `[]`    | Declared outputs to capture after execution           |
| `secrets` | `array[SecretDeclaration]` | No       | `[]`    | Step-level secrets (override flow-level if same name) |

### StepOutput Fields

| Field  | Type     | Required | Default | Description                                                  |
|--------|----------|----------|---------|--------------------------------------------------------------|
| `name` | `string` | ✅ Yes    | -       | Output identifier for template reference                     |
| `type` | `string` | ✅ Yes    | -       | Output type. Valid: `variable`, `file`                       |
| `path` | `string` | No*      | -       | File path relative to `state-dir`. *Required if `type: file` |

### SecretDeclaration Fields

| Field  | Type           | Required | Default | Description                    |
|--------|----------------|----------|---------|--------------------------------|
| `name` | `string`       | ✅ Yes    | -       | Secret identifier for template |
| `from` | `SecretSource` | ✅ Yes    | -       | Source configuration           |

### SecretSource Fields

| Field  | Type     | Required | Default | Description                                           |
|--------|----------|----------|---------|-------------------------------------------------------|
| `env`  | `string` | No*      | -       | Environment variable name. Takes precedence over file |
| `file` | `string` | No*      | -       | File path (whitespace trimmed). Fallback if env unset |

*At least one of `env` or `file` must be specified.

### Template Syntax

Reference outputs and secrets in commands using `${{ }}` syntax:

| Pattern                                | Description                        |
|----------------------------------------|------------------------------------|
| `${{ steps.<name>.outputs.<output> }}` | Reference a previous step's output |
| `${{ secrets.<name> }}`                | Reference a declared secret        |

**Note:** Secrets are resolved via templates only (not injected as environment variables). Secret values are masked with
`***` in yafe's logs.

### Environment Variables

| Variable           | Description                                                                |
|--------------------|----------------------------------------------------------------------------|
| `YAFE_OUTPUT`      | File path for writing `key=value` outputs (used with `type: variable`)     |
| `YAFE_STATE`       | State directory path for storing files (used with `type: file`)            |
| `YAFE_INPUT_<KEY>` | Job inputs passed via CLI (`-i key=value`) or REST API. Key is uppercased. |

### Examples

```yaml
# minimal
runs-on: host
steps:
    -   cmd: echo "hi"
        kind: shell
```

```yaml
# custom shell
steps:
    -   cmd: pwd && ls
        kind: shell
        shell: zsh
```

```yaml
# environment variables
steps:
    -   cmd: echo $DEBUG
        kind: shell
        env:
            - DEBUG=true
```

```yaml
# multi-line
steps:
    -   cmd: |
            echo "Line 1"
            echo "Line 2"
        kind: shell
```

```yaml
# with collecting outputs and re-using them in another step
state-dir: /tmp/custom-state    # optional, defaults to /tmp/yafe-<runID>
runs-on: host

steps:
    -   name: build                 # required, unique identifier
        kind: shell
        cmd: |
            echo "version=1.0.0" >> $YAFE_OUTPUT
            cp artifact.zip $YAFE_STATE/build/
        outputs:
            -   name: version
                type: variable
            -   name: artifact
                type: file
                path: build/artifact.zip   # relative to state-dir

    -   name: deploy
        kind: shell
        cmd: |
            echo "Deploying ${{ steps.build.outputs.version }}"
            unzip ${{ steps.build.outputs.artifact }}
```

```yaml
# with secrets
runs-on: host

secrets:
    -   name: api_key
        from:
            env: API_KEY

    -   name: db_password
        from:
            file: /run/secrets/db

steps:
    -   name: build
        kind: shell
        cmd: |
            curl -H "Authorization: Bearer ${{ secrets.api_key }}" https://api.example.com

    -   name: deploy
        kind: shell
        secrets:
            -   name: api_key           # overrides flow-level api_key for this step
                from:
                    env: DEPLOY_API_KEY
            -   name: deploy_token      # step-specific secret
                from:
                    file: /run/secrets/deploy
        cmd: |
            echo "Using deploy key: ${{ secrets.api_key }}"
            curl -H "X-Token: ${{ secrets.deploy_token }}" https://deploy.example.com
```

## Development and contribution

The most straight forward way to get started is by looking into available commands inside the `Makefile`.

For the full setup, you need the following tools:

- go (see minimum version in `go.mod`)
- make to execute commands of the `Makefile`
- Node and pnpm for the front-end

Quick start is to open two terminals and run each of the commands in one of them:

```shell
make dev-server
make dev-frontend
```

### Git workflow

The main branch is `main`. It's protected and only eligible users can push to it. Merge requests to protected branches
are safeguarded: they need review or at least a successful pipeline run to be merged.

- Use conventional commits as commit style and branch naming strategy, e.g., `feat/`, `fix/`, `refactor/`, `chore/`, or
  `ci/`
- **All** merge request commits should have a meaningful commit **title** and **message** stating the **why**
- Use atomic git commits, separate **preparatory** from **functional** commits to speed up review
- Avoid merging trunk back, use `git-rebase`

### Pipeline workflow

Pipeline runs

* on merge request change (open, new push, ...)
* on protected branches

This means you need to create a merge request to trigger a pipeline run. Without merge request, no build is triggered,
thus your code cannot be merged.

### Dependency updates

Dependency updates are handled by Renovate using the `renovate.json5` file. The base branch is `main`.

Major updates undergo manual review.

### Releases

1. Prepare a new MR to trunk with the following changes
   * Adjust and align versions
     * `flake.nix`: `version`
     * `main.go`: `Version`
     * `package.json`: `version`
   * Make sure `make clean dependencies checkstyle audit build-all test-coverage` is fine
   * Make sure `nix build` is fine (you need `nix` for it, update checksums in `flake.nix` if it fails)
   * Use `release/` as branch prefix and `release: prepare XYZ` as commit message
2. Merge to trunk 
3. Trigger the release job the semantic version which is inside the main trunk
4. Generate changelog and attach it to the release (use `git-cliff`)
5. Pull changes from trunk, prepare a new MR to trunk to prepare next version
   *  Adjust and align versions to the next semantic _patch_ version
   * `flake.nix`: `version`
   * `main.go`: `Version`
   * `package.json`: `version`
   * Use `release/` as branch prefix and `release: prepare next cycle...` as commit message
6. Merge to trunk