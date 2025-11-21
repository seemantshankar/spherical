# Git Worktrees Guide for Spherical

## Overview

Git worktrees allow you to maintain multiple working directories for a single Git repository, enabling parallel development on different branches without separate clones. This aligns perfectly with Spherical's constitution principle of **Git Best Practices** and feature branch workflow.

## Benefits for Spherical Development

### 1. Parallel Feature Development
- Work on multiple features simultaneously without switching branches
- Each worktree maintains its own working directory state
- No need to stash changes when switching between features

### 2. Testing & Code Review
- Keep main branch checked out in one worktree for quick reference
- Test multiple feature branches side-by-side
- Run integration tests across different branches simultaneously

### 3. Constitution Compliance
- Enforces feature branch isolation (Principle XI: Git Best Practices)
- Enables CLI-first testing during development (Principle III: CLI First Approach)
- Supports real-time task list updates across features (Principle V)

## Basic Git Worktree Commands

### Creating a Worktree

```bash
# Create a new worktree for an existing branch
git worktree add <path> <branch-name>

# Create a new worktree with a new branch
git worktree add <path> -b <new-branch-name>

# Example: Create worktree for feature 001-user-auth
git worktree add ../spherical-001-user-auth 001-user-auth

# Example: Create worktree with new branch
git worktree add ../spherical-002-payment -b 002-payment-integration
```

### Listing Worktrees

```bash
# List all worktrees
git worktree list

# Output format:
# /path/to/main      [main]
# /path/to/feature   [001-user-auth]
```

### Removing a Worktree

```bash
# Remove a worktree (must be done from main worktree)
git worktree remove <path>

# Force remove if branch is merged or worktree is locked
git worktree remove <path> --force

# Example:
git worktree remove ../spherical-001-user-auth
```

### Moving Between Worktrees

```bash
# Simply cd into the worktree directory
cd ../spherical-001-user-auth

# Each worktree has its own .git file pointing to the main repository
# No need for git checkout - just navigate to the directory
```

## Recommended Workflow for Spherical

### Setup: Main Worktree
Keep your main repository as the "main" worktree:

```bash
# Main repository (already exists)
cd /Users/seemant/Documents/Projects/spherical/Spherical
```

### Feature Development Pattern

```bash
# 1. Create feature branch (using your existing script)
.specify/scripts/bash/create-new-feature.sh 001-user-authentication

# 2. Create worktree for this feature
git worktree add ../spherical-001-user-auth 001-user-authentication

# 3. Work in the feature worktree
cd ../spherical-001-user-auth

# 4. Develop, test, commit (as per TDD principles)
# ... development work ...

# 5. When feature is complete and merged, remove worktree
cd /Users/seemant/Documents/Projects/spherical/Spherical
git worktree remove ../spherical-001-user-auth
```

### Multiple Features in Parallel

```bash
# Main worktree: main branch
cd /Users/seemant/Documents/Projects/spherical/Spherical

# Feature 1 worktree
git worktree add ../spherical-001-auth 001-user-authentication

# Feature 2 worktree
git worktree add ../spherical-002-payment 002-payment-integration

# Feature 3 worktree
git worktree add ../spherical-003-api 003-api-endpoints

# Now you can work on all three simultaneously:
# Terminal 1: cd ../spherical-001-auth && go test ./...
# Terminal 2: cd ../spherical-002-payment && go build
# Terminal 3: cd ../spherical-003-api && go run cmd/cli/main.go
```

## Cursor IDE Support for Git Worktrees

### Current Status (as of 2025)

✅ **Native Support**: Cursor has introduced native support for Git worktrees
- Allows multiple Cursor instances to work simultaneously
- Each worktree can be opened as a separate workspace
- Enables parallel development with multiple agents

⚠️ **Known Issues**:
- Some users report Cursor 2.0 Agent requesting write permissions for every file in worktrees
- This may require manual approval for each file modification
- Issue appears to be version-specific and may be resolved in updates

### Recommended Cursor Workflow

1. **Open Main Worktree in Cursor**
   ```bash
   cd /Users/seemant/Documents/Projects/spherical/Spherical
   cursor .
   ```

2. **Open Feature Worktree in Separate Cursor Window**
   ```bash
   cd ../spherical-001-user-auth
   cursor .
   ```

3. **Benefits**:
   - Each Cursor instance has its own file watcher
   - No conflicts between feature branches
   - Can use Cursor's Agent in both simultaneously (if not affected by the known issue)

### Best Practices with Cursor

1. **Separate Windows**: Open each worktree in its own Cursor window
   - Avoids confusion about which branch you're editing
   - Reduces risk of editing wrong branch files

2. **Window Naming**: Use descriptive window titles
   - Window 1: "Spherical - main"
   - Window 2: "Spherical - 001-user-auth"
   - Window 3: "Spherical - 002-payment"

3. **Agent Usage**: Test Cursor's Agent in worktrees
   - If the write-permission issue occurs, consider:
     - Using newer Cursor versions (check for updates)
     - Providing broader permissions when prompted
     - Working sequentially if parallel agent work causes issues

4. **Git Integration**: Cursor's Git integration works per-window
   - Each window shows Git status for its worktree branch
   - Source control panel reflects current worktree branch

## Integration with Spherical Constitution

### Principle I: Test Driven Development
- Each worktree maintains independent test state
- Run tests in parallel across multiple features
- No interference between feature test suites

### Principle II: Library First Approach
- Test libraries independently in separate worktrees
- Isolate library development from integration work
- Clear boundaries between feature libraries

### Principle III: CLI First Approach
- Test CLI interfaces in each worktree independently
- No need to rebuild or switch branches for CLI testing
- Faster iteration cycles

### Principle V: Real-Time Task List Updates
- Update task lists per feature in separate worktrees
- No merge conflicts on task documentation
- Clear progress tracking per feature

### Principle XI: Git Best Practices
- Enforces feature branch isolation
- Prevents accidental commits to wrong branch
- Maintains clean merge history

## Worktree Organization Structure

Recommended directory structure:

```
~/Documents/Projects/spherical/
├── Spherical/              # Main worktree (main branch)
│   ├── .git/
│   ├── .specify/
│   └── ... (main codebase)
│
├── spherical-001-user-auth/    # Feature 1 worktree
│   ├── .git -> Spherical/.git/worktrees/spherical-001-user-auth
│   └── ... (feature 1 codebase)
│
├── spherical-002-payment/      # Feature 2 worktree
│   ├── .git -> Spherical/.git/worktrees/spherical-002-payment
│   └── ... (feature 2 codebase)
│
└── ... (more feature worktrees)
```

## Practical Examples

### Example 1: Starting a New Feature

```bash
# From main worktree
cd /Users/seemant/Documents/Projects/spherical/Spherical

# Create feature branch using your script
.specify/scripts/bash/create-new-feature.sh user-authentication
# Output: BRANCH_NAME: 001-user-authentication

# Create worktree
git worktree add ../spherical-001-user-auth 001-user-authentication

# Open in Cursor (new window)
cd ../spherical-001-user-auth
cursor .

# Now develop following TDD:
# 1. Write tests first
# 2. Watch them fail
# 3. Implement feature
# 4. All tests pass
```

### Example 2: Reviewing PR While Working on New Feature

```bash
# Main worktree - review PR branch
cd /Users/seemant/Documents/Projects/spherical/Spherical
git fetch origin
git worktree add ../spherical-pr-review origin/pr-branch-name
cd ../spherical-pr-review
cursor .  # Review in separate window

# Feature worktree - continue your work
cd ../spherical-001-user-auth
cursor .  # Continue development in separate window
```

### Example 3: Testing Integration Between Features

```bash
# Feature A worktree
cd ../spherical-001-auth
go test ./...  # Run feature A tests

# Feature B worktree (in another terminal)
cd ../spherical-002-payment
go test ./...  # Run feature B tests

# Test integration (if features are related)
# Merge feature A into feature B's worktree temporarily
cd ../spherical-002-payment
git merge 001-auth
go test ./...
git reset --hard HEAD~1  # Reset after testing
```

## Limitations and Considerations

1. **Shared .git Directory**: All worktrees share the same `.git` directory
   - Ref updates are visible across worktrees
   - Some operations (like `git gc`) affect all worktrees

2. **File Locking**: Git locks some operations across worktrees
   - Cannot check out the same branch in multiple worktrees
   - Some Git operations require exclusive access

3. **Disk Space**: Each worktree has its own working directory
   - More disk space usage than single-checkout workflow
   - Trade-off for parallel development capability

4. **Complexity**: More worktrees = more management overhead
   - Keep track of which worktree you're in
   - Clean up merged worktrees regularly

## Cleanup and Maintenance

### Regular Cleanup

```bash
# List all worktrees
git worktree list

# Remove merged feature worktrees
git worktree remove ../spherical-001-user-auth  # After merge to main

# Clean up stale worktree references
git worktree prune
```

### Helper Script

Consider creating a helper script (`.specify/scripts/bash/manage-worktrees.sh`):

```bash
#!/usr/bin/env bash
# Manage git worktrees for Spherical project

case "$1" in
  list)
    git worktree list
    ;;
  add)
    BRANCH="$2"
    WORKTREE_PATH="../spherical-${BRANCH}"
    git worktree add "$WORKTREE_PATH" "$BRANCH"
    echo "Created worktree at $WORKTREE_PATH"
    ;;
  remove)
    WORKTREE_PATH="$2"
    git worktree remove "$WORKTREE_PATH"
    ;;
  prune)
    git worktree prune
    ;;
  *)
    echo "Usage: $0 {list|add <branch>|remove <path>|prune}"
    exit 1
    ;;
esac
```

## Troubleshooting

### Issue: "fatal: 'some/path' is already a working tree"

**Solution**: The path is already registered as a worktree. Remove it first:
```bash
git worktree remove <path>
# Or force remove:
git worktree remove <path> --force
```

### Issue: "fatal: 'branch-name' is already checked out"

**Solution**: You cannot check out the same branch in multiple worktrees. Either:
- Use a different branch name
- Remove the existing worktree using that branch
- Create the worktree with a new branch name

### Issue: Cursor asking for write permissions on all files

**Solution**: This is a known Cursor 2.0 Agent issue. Options:
1. Update Cursor to latest version
2. Grant broader permissions when prompted
3. Use Cursor sequentially instead of parallel if issue persists

## Further Reading

- [Git Worktree Documentation](https://git-scm.com/docs/git-worktree)
- [Git Worktree Tutorial](https://www.youtube.com/watch?v=oI631eCAQnQ)
- Cursor Community Forums for latest worktree support updates

---

**Version**: 1.0.0 | **Last Updated**: 2025-11-21 | **Status**: Active Guide

