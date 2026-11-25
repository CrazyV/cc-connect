# Fork Notes

This repository is a personal fork of [chenhg5/cc-connect](https://github.com/chenhg5/cc-connect).
It keeps the upstream architecture and adds one focused capability: choosing a recent
project from the `/dir` card, so work can be resumed from a phone.

## Background

When cc-connect is driven from Feishu/Lark on a phone, `/dir` used to show only the
current working directory and a short directory history. Projects opened on the
desktop were not listed at all, and absolute paths are hard to recall on a phone.
Leaving the desk therefore meant losing the ability to switch back to the project
being worked on.

The goal of this fork is narrow: expose the project lists that Codex and Claude Code
already maintain locally, and make switching to them a single tap.

## What Changed

- **Codex agent** (`agent/codex/recent_dirs.go`): read recent projects from the
  Codex Desktop project registry (`state_5.sqlite`) instead of inferring projects
  from raw thread `cwd` values.
- **Claude Code agent** (`agent/claudecode/recent_dirs.go`): derive recent projects
  from Claude Code project session directories, reading the real working directory
  from session transcripts because directory names are encoded lossily.
- **Core** (`core/`): add an agent-scoped `RecentWorkDir` capability, render recent
  projects in the `/dir` card, handle card navigation, and add i18n strings.
- **Feishu platform** (`platform/feishu/`): after a project switch, return a short
  toast together with the updated card, so the action has immediate feedback without
  adding a chat message.
- **Docs and tests**: README, usage docs, unit tests, and CUJ coverage updated.

The lists are agent-scoped: Codex and Claude Code never mix their projects, and
agents without this capability keep the previous history-only behavior.

## Usage

```text
/dir          Show the current directory, recent projects, and history
/dir <path>   Switch to a path
/dir <number> Switch by history index
/dir -        Switch back to the previous directory
```

On Feishu, tapping a project in the `/dir` card updates the card in place and shows
a toast such as `✅ Switched to my-project`.

## Verification

```bash
go build ./...
go test ./core ./agent/codex ./agent/claudecode ./platform/feishu
go test ./core -run TestCUJ
```

CI on `main` runs lint, unit tests, smoke tests, blackbox smoke tests, regression
tests, and performance benchmarks.

## Syncing with Upstream

```bash
git fetch upstream
git merge upstream/main
```

`upstream` points at `chenhg5/cc-connect`; `origin` points at this fork.

## License

MIT. Upstream copyright is preserved in [LICENSE](LICENSE).

---

## 中文说明

本仓库是 [chenhg5/cc-connect](https://github.com/chenhg5/cc-connect) 的个人 fork，
只增加了一个聚焦的能力：在 `/dir` 卡片里直接选择最近项目，方便手机端继续工作。

### 背景

在手机上通过飞书使用 cc-connect 时，`/dir` 只能看到当前工作目录和一小段目录历史，
桌面端打开过的项目不会出现，而手机上也记不住项目的绝对路径。人离开电脑后，
就无法切回正在开发的项目继续对话。

### 改动内容

- **Codex**：从 Codex Desktop 的项目注册表（`state_5.sqlite`）读取最近项目，
  不再把会话 `cwd` 当作项目列表。
- **Claude Code**：从 Claude Code 的项目会话目录推导最近项目，并从会话记录中
  读取真实工作目录。
- **core**：新增按 Agent 隔离的 `RecentWorkDir` 能力，在 `/dir` 卡片中渲染最近项目，
  处理卡片点击切换，并补齐多语言文案。
- **飞书**：项目切换成功后，卡片原位更新并返回 Toast 提示，不额外发送聊天消息。
- **文档与测试**：同步更新 README、使用文档、单元测试与 CUJ 覆盖。

Codex 与 Claude Code 的项目列表互不混用；未实现该能力的 Agent 保持原有的
“仅目录历史”行为。

### 使用

```text
/dir          查看当前目录、最近项目与历史
/dir <路径>   切换到指定目录
/dir <序号>   按历史序号切换
/dir -        返回上一个目录
```

### 同步上游

```bash
git fetch upstream
git merge upstream/main
```
