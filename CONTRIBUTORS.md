# CONTRIBUTORS.md

Maintainer notes for `gh-actions-watcher`.

## Prerequisites

- Go `1.26+`
- Node `18+`
- npm account with publish rights for `@amxv/gh-actions-watcher`
- GitHub repo admin access

## Local development

```bash
make check
make build
./dist/gha-watch --help
```

Install locally:

```bash
make install-local
gha-watch --help
```

## Release process

1. Ensure `main` is green:

```bash
make check
```

2. Prepare release tag:

```bash
make release-tag VERSION=0.1.0
```

3. GitHub Actions `release` workflow runs automatically:

- quality checks
- cross-platform binary build
- GitHub release publish
- npm publish

## Required GitHub secret

- `NPM_TOKEN`: npm automation token with publish rights for `@amxv/gh-actions-watcher`.

Set via GitHub CLI:

```bash
gh secret set NPM_TOKEN --repo amxv/gh-actions-watcher
```

## npm token setup

Create token at npm:

- Profile -> Access Tokens -> Create New Token
- Use an automation/granular token scoped to required package/org

Validate auth locally:

```bash
npm whoami
```
