# actions-watcher

Fail-fast GitHub Actions watcher written in Go.

The CLI monitors one or more workflow run IDs and exits immediately when any job or step reaches a failing conclusion (`failure`, `cancelled`, `timed_out`, etc.).

## Install

```bash
npm i -g actions-watcher
actions-watcher --help
```

## Usage

```bash
actions-watcher watch [--repo owner/repo] [--interval seconds] [--token token] RUN_ID [RUN_ID ...]

# shorthand (without explicit subcommand)
actions-watcher [--repo owner/repo] [--interval seconds] RUN_ID [RUN_ID ...]
```

Authentication is resolved in this order:

1. `--token`
2. `GH_TOKEN` or `GITHUB_TOKEN`
3. `gh auth token`

Repository slug is resolved in this order:

1. `--repo`
2. `GITHUB_REPOSITORY`
3. `git remote get-url origin`

## Examples

```bash
# Watch a single run
actions-watcher watch 1934567890

# Watch multiple runs in one command
actions-watcher watch --repo amxv/computer-mcp 1934567890 1934567999

# Faster polling for local debugging
actions-watcher --interval 1.0 --repo amxv/computer-mcp 1934567890
```

## Development

```bash
make check
make build
./dist/actions-watcher --help
```

Install locally:

```bash
make install-local
actions-watcher --help
```

## Release

Pushing a tag `vX.Y.Z` triggers `.github/workflows/release.yml`:

1. test + vet + node checks
2. cross-platform binary build
3. GitHub release asset upload
4. npm publish with the tag version

Required GitHub secret:

- `NPM_TOKEN`
