# Task #010: 工作流守卫 Hook

## Status: done

## 需求

Claude Code 完成 task 时有时跳过工作流步骤（task 文件、codex review），需要用 PreToolUse hook 硬性拦截违规 commit。

## 设计

- PreToolUse hook 拦截 `git commit`，检查：1) docs/tasks/ 有 in-progress 的 task 文件；2) `.codex-review-done` 标记文件存在
- PostToolUse hook 检测 `codex review` 执行后创建标记文件
- PostToolUse hook 在 commit 后清除标记文件
- CLAUDE.md 添加 Task Completion Self-Check 清单

## 涉及文件

- `scripts/hooks/pre-commit-check.sh`（新建）
- `scripts/hooks/post-codex-review.sh`（新建）
- `scripts/hooks/clear-review-marker.sh`（新建）
- `.claude/settings.local.json`（修改，添加 hooks）
- `.gitignore`（修改，忽略 `.codex-review-done`）
- `CLAUDE.md`（修改，添加自检清单）

## TODO

- [x] 创建 3 个 hook 脚本
- [x] 更新 .gitignore
- [x] 更新 settings.local.json
- [x] 更新 CLAUDE.md
- [x] 手动测试验证
- [ ] codex review
- [ ] commit + 更新 backlog

## 中断记录

（无）

## 完成标准

- [x] hook 拦截无 task 文件的 commit
- [x] hook 拦截未 codex review 的 commit
- [x] 非 commit 命令不受影响
- [ ] commit + codex review
