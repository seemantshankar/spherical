#!/usr/bin/env bash
# Git Worktree Management Script for Spherical
#
# Provides convenient CLI commands for managing git worktrees aligned with
# Spherical's constitution principles (CLI First, Git Best Practices)
#
# Usage: ./manage-worktrees.sh {command} [arguments]

set -e

# Source common functions
SCRIPT_DIR="$(CDPATH="" cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/common.sh"

# Get repository root
REPO_ROOT=$(get_repo_root)

# Worktree directory pattern
WORKTREE_PREFIX="spherical"

show_usage() {
    cat << EOF
Git Worktree Manager for Spherical

Usage: $0 {command} [arguments]

Commands:
    list                    List all active worktrees
    add <branch>            Create worktree for branch at ../${WORKTREE_PREFIX}-<branch>
    add <branch> <path>     Create worktree for branch at specified path
    remove <path>           Remove worktree at path
    remove <branch>         Remove worktree for branch (auto-detects path)
    prune                   Remove stale worktree references
    status                  Show status of all worktrees
    help                    Show this help message

Examples:
    $0 list
    $0 add 001-user-auth
    $0 add 001-user-auth ../my-custom-path
    $0 remove ../${WORKTREE_PREFIX}-001-user-auth
    $0 remove 001-user-auth
    $0 prune

Notes:
    - Worktrees are created in parent directory of main repository
    - Branch names should follow Spherical convention: ###-feature-name
    - All operations must be run from main repository directory
EOF
}

list_worktrees() {
    if ! has_git; then
        echo "ERROR: Not in a git repository" >&2
        exit 1
    fi
    
    echo "Active Git Worktrees:"
    echo "===================="
    git worktree list
}

add_worktree() {
    local branch="$1"
    local path="${2:-}"
    
    if ! has_git; then
        echo "ERROR: Not in a git repository" >&2
        exit 1
    fi
    
    if [[ -z "$branch" ]]; then
        echo "ERROR: Branch name required" >&2
        echo "Usage: $0 add <branch> [path]" >&2
        exit 1
    fi
    
    # Auto-generate path if not provided
    if [[ -z "$path" ]]; then
        # Remove any path separators from branch name for safety
        local safe_branch=$(echo "$branch" | sed 's/[^a-zA-Z0-9-]/-/g')
        path="../${WORKTREE_PREFIX}-${safe_branch}"
    fi
    
    # Check if branch exists
    if ! git show-ref --verify --quiet refs/heads/"$branch"; then
        echo "Branch '$branch' does not exist. Creating new branch..."
        git worktree add -b "$branch" "$path"
    else
        echo "Branch '$branch' exists. Adding worktree..."
        git worktree add "$path" "$branch"
    fi
    
    echo "✓ Created worktree at: $path"
    echo "  Branch: $branch"
    echo ""
    echo "To open in Cursor:"
    echo "  cd $path && cursor ."
}

remove_worktree() {
    local target="$1"
    
    if ! has_git; then
        echo "ERROR: Not in a git repository" >&2
        exit 1
    fi
    
    if [[ -z "$target" ]]; then
        echo "ERROR: Path or branch name required" >&2
        echo "Usage: $0 remove <path|branch>" >&2
        exit 1
    fi
    
    # If target looks like a branch name (contains numbers and dashes), find worktree
    if [[ "$target" =~ ^[0-9]+-[a-zA-Z0-9-]+$ ]]; then
        # Try to find worktree with this branch
        local worktree_info=$(git worktree list | grep "\[$target\]" || true)
        if [[ -z "$worktree_info" ]]; then
            echo "ERROR: No worktree found for branch '$target'" >&2
            exit 1
        fi
        # Extract path (first field before spaces)
        target=$(echo "$worktree_info" | awk '{print $1}')
        echo "Found worktree at: $target"
    fi
    
    # Validate path exists as worktree
    if ! git worktree list | grep -q "$target"; then
        echo "ERROR: '$target' is not a registered worktree" >&2
        echo "Run '$0 list' to see active worktrees" >&2
        exit 1
    fi
    
    # Check if worktree has uncommitted changes
    local branch=$(git -C "$target" rev-parse --abbrev-ref HEAD 2>/dev/null || echo "")
    if [[ -n "$branch" ]]; then
        if ! git -C "$target" diff --quiet || ! git -C "$target" diff --cached --quiet; then
            echo "WARNING: Worktree has uncommitted changes" >&2
            echo "Branch: $branch" >&2
            read -p "Remove anyway? (y/N): " -n 1 -r
            echo
            if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                echo "Aborted."
                exit 0
            fi
        fi
    fi
    
    git worktree remove "$target"
    echo "✓ Removed worktree: $target"
}

prune_worktrees() {
    if ! has_git; then
        echo "ERROR: Not in a git repository" >&2
        exit 1
    fi
    
    echo "Pruning stale worktree references..."
    git worktree prune
    echo "✓ Pruning complete"
}

show_status() {
    if ! has_git; then
        echo "ERROR: Not in a git repository" >&2
        exit 1
    fi
    
    echo "Worktree Status:"
    echo "==============="
    echo ""
    
    # Main repository
    local main_branch=$(git rev-parse --abbrev-ref HEAD)
    echo "Main Worktree:"
    echo "  Path: $REPO_ROOT"
    echo "  Branch: $main_branch"
    if ! git diff --quiet || ! git diff --cached --quiet; then
        echo "  Status: ⚠️  Has uncommitted changes"
    else
        echo "  Status: ✓ Clean"
    fi
    echo ""
    
    # List all worktrees
    local worktrees=$(git worktree list --porcelain | grep -E "^worktree" | awk '{print $2}')
    
    if [[ -z "$worktrees" ]]; then
        echo "No additional worktrees"
        return
    fi
    
    echo "Additional Worktrees:"
    while IFS= read -r worktree_path; do
        if [[ "$worktree_path" == "$REPO_ROOT" ]]; then
            continue
        fi
        
        local branch=$(git -C "$worktree_path" rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
        echo "  Path: $worktree_path"
        echo "  Branch: $branch"
        
        if [[ -d "$worktree_path" ]]; then
            if git -C "$worktree_path" diff --quiet 2>/dev/null && git -C "$worktree_path" diff --cached --quiet 2>/dev/null; then
                echo "  Status: ✓ Clean"
            else
                echo "  Status: ⚠️  Has uncommitted changes"
            fi
        else
            echo "  Status: ✗ Directory missing (stale reference)"
        fi
        echo ""
    done <<< "$worktrees"
}

# Main command dispatcher
main() {
    local command="${1:-help}"
    
    case "$command" in
        list)
            list_worktrees
            ;;
        add)
            if [[ -z "$2" ]]; then
                echo "ERROR: Branch name required" >&2
                show_usage
                exit 1
            fi
            add_worktree "$2" "$3"
            ;;
        remove|rm)
            if [[ -z "$2" ]]; then
                echo "ERROR: Path or branch name required" >&2
                show_usage
                exit 1
            fi
            remove_worktree "$2"
            ;;
        prune)
            prune_worktrees
            ;;
        status|stat)
            show_status
            ;;
        help|--help|-h)
            show_usage
            ;;
        *)
            echo "ERROR: Unknown command: $command" >&2
            echo ""
            show_usage
            exit 1
            ;;
    esac
}

main "$@"

