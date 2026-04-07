#!/bin/bash
# PostToolUse hook: clear codex review marker after git commit succeeds

PROJECT_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

# Read tool input JSON from stdin
INPUT=$(cat)

# Extract the command string
COMMAND=$(echo "$INPUT" | python3 -c "import sys,json; data=json.load(sys.stdin); print(data.get('tool_input',{}).get('command',''))" 2>/dev/null)

# Clear marker after commit, but only if commit likely succeeded
# Check if staging area is clean (no staged changes = commit went through)
if echo "$COMMAND" | grep -q 'git commit'; then
  STAGED=$(git -C "$PROJECT_ROOT" diff --cached --quiet 2>/dev/null; echo $?)
  if [ "$STAGED" = "0" ]; then
    rm -f "$PROJECT_ROOT/.codex-review-done"
  fi
fi

exit 0
