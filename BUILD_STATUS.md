# Voxi MVP Build Status

## Completed

- Reviewed `prd.md` and `trd.md`
- Created implementation plan and milestone breakdown
- Initialized Go module
- Created project scaffolding and build files
- Implemented initial Go CLI commands: `voxi daemon`, `voxi toggle`, `voxi status`, `voxi doctor`
- Implemented daemon RPC server, state machine, notifications, structured logging, and fallback-capable pipeline orchestration
- Implemented Python worker package with fake and real adapter paths, Unix socket server, and health reporting
- Added Go and Python automated tests for state safety, IPC, fallback behavior, doctor checks, and worker restart recovery
- Added reusable command shims under `testdata/bin` for local smoke testing
- Added Fedora-aware idempotent `scripts/setup.sh`
- Expanded README with quickstart, architecture, testing, troubleshooting, and GNOME shortcut instructions
- Validated manual happy-path and clipboard-fallback smoke runs with fake worker/shim tooling
- Verified `voxi doctor` actionable output on the current VM

## Decisions made

- Repository-local `IMPLEMENTATION_PLAN.md` mirrors the approved plan
- Use a thin CLI with Unix socket JSON RPC to the daemon
- Use fake-driven tests by default so automated runs do not require models, GNOME, or GPU access
- Use a warm Python worker supervisor in the Go daemon, with automatic restart on socket/process failure
- Use `pw-record` for PipeWire capture and normalize output to `pcm_s16le`
- Install the Python worker into a project-local virtualenv and point `worker_python` at that interpreter in the generated config

## Blockers and resolution

- Python `venv` bootstrap is unavailable in this VM because `ensurepip` is missing
  - resolved for local testing by installing `pytest` and the editable worker package into the user site with `python3 -m pip install --user --break-system-packages ...`
- GitHub API pull-request creation returned HTTP 403 from this environment
  - captured the full PR title/body in `PR_REPORT.md` so the report is preserved even though the PR could not be created programmatically
