#!/bin/bash
# PostToolUse hook: create marker file after codex review is executed

PROJECT_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

# Read tool input JSON from stdin
INPUT=$(cat)

# Extract the command string
COMMAND=$(echo "$INPUT" | python3 -c "import sys,json; data=json.load(sys.stdin); print(data.get('tool_input',{}).get('command',''))" 2>/dev/null)

# Detect codex review invocations
if echo "$COMMAND" | grep -qE 'codex\s+review|npx\s+@openai/codex\s+review'; then
  touch "$PROJECT_ROOT/.codex-review-done"
fi

exit 0
