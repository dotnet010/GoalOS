# GoalOS Git 操作规范

> **规则层级**: 最高优先级。所有 AI 必须在执行任何 git 操作前先读取本文件。
> **违反后果**: 直接中断操作，报告用户。

---

## 规则 1：禁止删除 .git 仓库

**`rm -rf .git` 或任何删除 `.git` 目录的操作——绝对禁止。**

- `.git` 目录包含全部提交历史、分支、标签
- 删除后不可恢复（不进废纸篓）
- 唯一的例外：用户**明确书面授权**删除

## 规则 2：Git 主目录

**项目 Git 仓库位于 `GoalOS/` 目录下。**

- 主目录 `/Users/haochen/work/workspace/pi2/` 不是 git 仓库
- 所有 git 操作必须在 `GoalOS/` 目录内执行：`git -C /Users/haochen/work/workspace/pi2/GoalOS <command>`
- 不在主目录或其他子目录初始化 git

## 规则 3：远程仓库地址

**Remote: `https://github.com/dotnet010/GoalOS`**

- `git remote -v` 必须显示此地址
- 如需重新配置：`git remote add origin https://github.com/dotnet010/GoalOS.git`

## 规则 4：Git 操作必须先保持完整信息

执行以下操作前必须检查：

| 操作 | 前置检查 |
|------|---------|
| commit | `git status` — 确认只包含项目文件。排除 `.DS_Store`、个人文件、缓存 |
| push | `git log --oneline -5` — 确认提交历史正确。`git diff origin/main` — 确认变更范围 |
| branch | `git branch -a` — 确认当前分支。不在 main 上直接 commit 新功能 |
| force push | **禁止**。永远不使用 `--force` |
| rebase | **禁止**。永远不 rebase 已推送的分支 |
| 删除分支 | 确认分支已合并到 main |

## 规则 5：项目文件范围

只提交以下目录的文件：
- `GoalOS/internal/`
- `GoalOS/cmd/`
- `GoalOS/pkg/`
- `GoalOS/scripts/`
- `GoalOS/*.md`（规范文档）
- `GoalOS/*.yaml`（配置文件）
- `GoalOS/Makefile`
- `GoalOS/go.mod`、`GoalOS/go.sum`
- `.github/`

排除：
- `.DS_Store`
- 个人文件、临时文件、缓存
- 审计记录
- AI 提示词

---

**最后更新**: 2026-06-30
**签名**: Jobs 裁定——本规范为项目基础规范。任何 AI 违反→立即停止操作并报告。
