---
name: pr-workflow
description: Create feature branch with git worktree, push, open PR via gh, review diff, merge, and cleanup
---

## PR Workflow

Use this workflow for any feature, fix, or refactoring that should go through code review before merging into `develop`.

## Steps

### 1. Create a git worktree

Isolate work from the main checkout to avoid mixing changes:

```bash
git worktree add ../telemusic-<feature> -b feature/<name> develop
```

All edits happen in the worktree directory, not the main repo.

### 2. Implement, build, verify

Make changes in the worktree. Before committing:

```bash
go build ./cmd/bot
go fmt ./...
go vet ./...
```

### 3. Commit

Use conventional commit messages (`feat:`, `fix:`, `refactor:`, `docs:`, `chore:`). Keep the subject concise and add a body listing key changes:

```bash
git add <files>
git commit -m "feat: short description

- Detail one
- Detail two"
```

Do NOT include build artifacts or binaries. Check `git status` before committing.

### 4. Push and create PR

```bash
git push -u origin feature/<name>
```

Create the PR targeting `develop` with a structured body:

```bash
gh pr create --base develop --title "feat: short title" --body "$(cat <<'EOF'
## Summary
- Bullet points describing what and why

## Changes
- `path/to/file.go` -- what changed

## Testing
- How it was verified
EOF
)"
```

### 5. Review before merging

Always review the full diff before merging:

```bash
gh pr diff <number>
```

Check for: leftover debug code, binary files, secrets, correct escaping, missing error handling.

### 6. Merge and cleanup

```bash
gh pr merge <number> --merge --delete-branch
```

Then clean up the worktree and sync:

```bash
# Stop any running containers from the worktree
docker compose down  # (if applicable, from worktree dir)

# Remove worktree and pull
git worktree remove ../telemusic-<feature>
git stash  # if needed
git pull origin develop
git stash pop  # if needed
```
