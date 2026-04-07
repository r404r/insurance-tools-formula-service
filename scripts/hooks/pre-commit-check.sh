#!/bin/bash
# PreToolUse hook: intercept git commit and validate workflow prerequisites
# Exit 0 = allow, Exit 2 = block (stderr shown to Claude)

PROJECT_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

# Read tool input JSON from stdin
INPUT=$(cat)

# Extract the command string
COMMAND=$(echo "$INPUT" | python3 -c "import sys,json; data=json.load(sys.stdin); print(data.get('tool_input',{}).get('command',''))" 2>/dev/null)

# Only check git commit commands
if ! echo "$COMMAND" | grep -q 'git commit'; then
  exit 0
fi

ERRORS=""

# Check 1: in-progress task file exists in docs/tasks/
# Support both formats: "## Status: in-progress" and "**Status**: in-progress"
TASK_FILES=$(grep -rlE '(## Status: in-progress|\*\*Status\*\*: in-progress)' "$PROJECT_ROOT/docs/tasks/"*.md 2>/dev/null | grep -v TEMPLATE.md)
if [ -z "$TASK_FILES" ]; then
  ERRORS="${ERRORS}❌ 错误：docs/tasks/ 中没有找到 Status 为 in-progress 的任务文件。\n   请先创建任务文件再提交。参考模板：docs/tasks/TEMPLATE.md\n\n"
fi

# Check 2: codex review marker exists
if [ ! -f "$PROJECT_ROOT/.codex-review-done" ]; then
  ERRORS="${ERRORS}❌ 错误：未检测到 codex review 的执行记录。\n   请先运行 /codex review 再提交。\n\n"
fi

if [ -n "$ERRORS" ]; then
  echo -e "\n🚫 Commit 被阻止 — 工作流前提条件未满足：\n\n$ERRORS" >&2
  exit 2
fi

exit 0
