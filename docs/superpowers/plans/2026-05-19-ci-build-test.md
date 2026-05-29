# CI Build And Test Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add GitHub Actions CI that runs build and test checks whenever a pull request targets `main` and whenever code is pushed to `main` after merge.

**Architecture:** Keep the executable CI contract in `Makefile`, then have GitHub Actions call those same commands. Add one workflow with separate `build` and `test` jobs so failures show whether compile or tests broke. The test job enables the existing testcontainers migration test with `RUN_INTEGRATION_TESTS=1`, and verifies Docker availability before running tests.

**Tech Stack:** GitHub Actions, Go from `go.mod`, Make, Docker, testcontainers, `actions/checkout@v6`, `actions/setup-go@v6`.

---

### Task 1: Makefile CI Commands

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Verify the current build command is missing**

Run:

```bash
make build
```

Expected: FAIL with output containing:

```text
No rule to make target `build'
```

- [ ] **Step 2: Replace `Makefile` with CI-friendly targets**

Replace the full file with:

```makefile
APP_NAME := hunch-server
SQLC ?= go run github.com/sqlc-dev/sqlc/cmd/sqlc@latest
GOOSE ?= go run github.com/pressly/goose/v3/cmd/goose@latest

.PHONY: help fmt build test test-ci run tidy sqlc migrate-up migrate-down compose-up compose-down

help:
	@printf "%s\n" "Targets: fmt build test test-ci run tidy sqlc migrate-up migrate-down compose-up compose-down"

fmt:
	gofmt -w ./cmd ./internal

build:
	go build -trimpath -o bin/$(APP_NAME) ./cmd/api

test:
	go test ./...

test-ci:
	RUN_INTEGRATION_TESTS=1 go test ./...

run:
	go run ./cmd/api

tidy:
	go mod tidy

sqlc:
	$(SQLC) generate

migrate-up:
	$(GOOSE) -dir db/migrations postgres "$$DATABASE_URL" up

migrate-down:
	$(GOOSE) -dir db/migrations postgres "$$DATABASE_URL" down

compose-up:
	docker compose up -d postgres prometheus

compose-down:
	docker compose down
```

- [ ] **Step 3: Run the build target**

Run:

```bash
make build
```

Expected: PASS and creates `bin/hunch-server`.

- [ ] **Step 4: Run the default test target**

Run:

```bash
make test
```

Expected: PASS. The migration integration test may report a skip with this text:

```text
set RUN_INTEGRATION_TESTS=1 to run testcontainers migration test
```

- [ ] **Step 5: Run the CI test target**

Run:

```bash
make test-ci
```

Expected: PASS, including `internal/db` migration test execution through testcontainers. If Docker Desktop is not running locally, start Docker Desktop and run the same command again.

- [ ] **Step 6: Commit the Makefile contract**

Run:

```bash
git add Makefile
git commit -m "build: add CI make targets"
```

Expected: PASS and creates one commit containing only the `Makefile` change.

### Task 2: GitHub Actions Workflow

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Verify no CI workflow exists yet**

Run:

```bash
test ! -f .github/workflows/ci.yml
```

Expected: PASS with no output.

- [ ] **Step 2: Create the workflow directory**

Run:

```bash
mkdir -p .github/workflows
```

Expected: PASS with no output.

- [ ] **Step 3: Create `.github/workflows/ci.yml`**

Create `.github/workflows/ci.yml` with:

```yaml
name: CI

on:
  pull_request:
    branches:
      - main
    types:
      - opened
      - synchronize
      - reopened
      - ready_for_review
  push:
    branches:
      - main
  workflow_dispatch:

permissions:
  contents: read

concurrency:
  group: ci-${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

jobs:
  build:
    name: Build
    runs-on: ubuntu-latest
    timeout-minutes: 10

    steps:
      - name: Check out repository
        uses: actions/checkout@v6

      - name: Set up Go
        uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
          cache: true
          cache-dependency-path: go.sum

      - name: Build API
        run: make build

  test:
    name: Test
    runs-on: ubuntu-latest
    timeout-minutes: 20

    steps:
      - name: Check out repository
        uses: actions/checkout@v6

      - name: Set up Go
        uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
          cache: true
          cache-dependency-path: go.sum

      - name: Verify Docker is available
        run: docker version

      - name: Run tests
        run: make test-ci
```

- [ ] **Step 4: Verify the workflow file exists**

Run:

```bash
test -f .github/workflows/ci.yml
```

Expected: PASS with no output.

- [ ] **Step 5: Inspect the workflow content**

Run:

```bash
sed -n '1,220p' .github/workflows/ci.yml
```

Expected output:

```yaml
name: CI

on:
  pull_request:
    branches:
      - main
    types:
      - opened
      - synchronize
      - reopened
      - ready_for_review
  push:
    branches:
      - main
  workflow_dispatch:

permissions:
  contents: read

concurrency:
  group: ci-${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

jobs:
  build:
    name: Build
    runs-on: ubuntu-latest
    timeout-minutes: 10

    steps:
      - name: Check out repository
        uses: actions/checkout@v6

      - name: Set up Go
        uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
          cache: true
          cache-dependency-path: go.sum

      - name: Build API
        run: make build

  test:
    name: Test
    runs-on: ubuntu-latest
    timeout-minutes: 20

    steps:
      - name: Check out repository
        uses: actions/checkout@v6

      - name: Set up Go
        uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
          cache: true
          cache-dependency-path: go.sum

      - name: Verify Docker is available
        run: docker version

      - name: Run tests
        run: make test-ci
```

- [ ] **Step 6: Commit the workflow**

Run:

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add build and test workflow"
```

Expected: PASS and creates one commit containing only `.github/workflows/ci.yml`.

### Task 3: README CI Documentation

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add the CI badge below the title**

Change the first lines of `README.md` from:

```markdown
# Hunch Server

Go server foundation for Hunch.
```

to:

```markdown
# Hunch Server

[![CI](https://github.com/dev-owen/hunch-server/actions/workflows/ci.yml/badge.svg)](https://github.com/dev-owen/hunch-server/actions/workflows/ci.yml)

Go server foundation for Hunch.
```

- [ ] **Step 2: Update the development command block**

Change the `## Development` command block from:

```markdown
```bash
make fmt
make test
make sqlc
make migrate-up
```
```

to:

````markdown
```bash
make fmt
make build
make test
make test-ci
make sqlc
make migrate-up
```
````

- [ ] **Step 3: Add the CI behavior section**

Add this section immediately after the `## Development` command block:

```markdown
## CI

GitHub Actions runs `make build` and `make test-ci` for pull requests targeting `main`, pushes to `main`, and manual workflow dispatches. The CI test target sets `RUN_INTEGRATION_TESTS=1`, so Docker must be available for the testcontainers migration test.
```

- [ ] **Step 4: Inspect the README changes**

Run:

```bash
sed -n '1,180p' README.md
```

Expected output includes:

```markdown
# Hunch Server

[![CI](https://github.com/dev-owen/hunch-server/actions/workflows/ci.yml/badge.svg)](https://github.com/dev-owen/hunch-server/actions/workflows/ci.yml)
```

Expected output also includes:

```markdown
make build
make test-ci
```

Expected output also includes:

```markdown
GitHub Actions runs `make build` and `make test-ci` for pull requests targeting `main`, pushes to `main`, and manual workflow dispatches.
```

- [ ] **Step 5: Commit the README update**

Run:

```bash
git add README.md
git commit -m "docs: document CI checks"
```

Expected: PASS and creates one commit containing only `README.md`.

### Task 4: End-To-End Verification

**Files:**
- Verify: `Makefile`
- Verify: `.github/workflows/ci.yml`
- Verify: `README.md`

- [ ] **Step 1: Confirm the final diff**

Run:

```bash
git diff --stat HEAD~3..HEAD
```

Expected output includes:

```text
 .github/workflows/ci.yml
 Makefile
 README.md
```

- [ ] **Step 2: Check formatting-sensitive whitespace**

Run:

```bash
git diff --check HEAD~3..HEAD
```

Expected: PASS with no output.

- [ ] **Step 3: Run local build**

Run:

```bash
make build
```

Expected: PASS.

- [ ] **Step 4: Run local unit tests**

Run:

```bash
make test
```

Expected: PASS.

- [ ] **Step 5: Run local CI tests**

Run:

```bash
make test-ci
```

Expected: PASS. This command starts a temporary `pgvector/pgvector:pg16` container through testcontainers.

- [ ] **Step 6: Push and confirm GitHub Actions**

Run:

```bash
git push
```

Expected: PASS.

Then run:

```bash
gh run list --workflow CI --limit 1
```

Expected output includes one recent run with status `completed` and conclusion `success`.

### Self-Review

**Spec coverage:** PR checks are covered by `pull_request` targeting `main`. Merge checks are covered by `push` to `main`, because a merged PR writes commits to the base branch. Build coverage is `make build`. Test coverage is `make test-ci`, which includes existing integration coverage by setting `RUN_INTEGRATION_TESTS=1`.

**Placeholder scan:** The plan contains exact file paths, commands, file contents, and expected outputs. It does not rely on undefined functions or deferred implementation text.

**Type consistency:** The workflow calls `make build` and `make test-ci`, both defined in Task 1. README commands match the Makefile targets. The workflow badge path matches `.github/workflows/ci.yml`.

### References Checked

- GitHub Actions Go guide: `actions/setup-go` is the recommended action for Go workflows.
- `actions/setup-go` README: `go-version-file` supports reading the Go version from `go.mod`.
- GitHub Actions event docs: `pull_request` and `push` events are supported workflow triggers.
