<div align="center">

# Code Kanban

[中文](README.zh-CN.md) | [English](README.md)

One panel, 10× faster coding.

[Core Features](#core-features) • [Quick Start](#quick-start) • [Development Guide](#development-guide) • [About](#about)

![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)
![Vue](https://img.shields.io/badge/Vue-3.5-4FC08D?logo=vue.js)
![TypeScript](https://img.shields.io/badge/TypeScript-5.8-3178C6?logo=typescript)
![License](https://img.shields.io/badge/license-Apache--2.0-green)

![Preview](docs/preview-en.png)

</div>

## Quick Start

Try it instantly:

```bash
npx codekanban
# or
npm install -g codekanban
```

## Core Features

- **Ready to Use**: Single file, local database, double-click to use
- **Terminal Hub**: All your projects, terminals and coding agents in one place (Hotkey: `)
- **Coding Agent Monitor**: Auto-detect Claude Code/Codex running status in terminals, notify when tasks complete
- **Task Kanban**: Visualize development task management
- **Worktree Management**: Create worktree branches, support rebase and merge back to main branch
- **Notes**: Simple multi-tab notes for quick problem tracking (Hotkey: 1)

## Development Guide

### Requirements
- **Node.js**: v20.19.0+ or v22.12.0+
- **Go**: 1.24.6+
- **Package Manager**: pnpm (recommended)

### Install Dependencies

**Frontend Dependencies**:
```bash
cd ui
pnpm install
```

**Backend Dependencies**:
```bash
go mod tidy
```

### Development Mode

**Frontend Development Server**:
```bash
cd ui
pnpm dev
```
Access at: `http://localhost:5173`

**Backend Development Server**:
```bash
go run . # Note: After first run, config.yaml will be generated with port 3007. Due to conflicts with the production version, they cannot run simultaneously. It's recommended to change to 3005. The following assumes it has been changed.
```
- Server Port: `http://localhost:3005`
- OpenAPI Documentation: `http://localhost:3005/docs`
- Health Check: `http://localhost:3005/api/v1/health`

**Optional Parameters**:
- `-m` or `--migrate`: Force database migration
- `-i` or `--install`: Install as system service
- `--uninstall`: Uninstall system service

### Production Build

**Full Build** (Recommended):
```bash
python build.py
```
This script will automatically complete the following steps:
1. Build frontend (`pnpm build`)
2. Copy frontend artifacts to `static/` directory
3. Build Go executable (with optimization)

**Manual Build**:
```bash
# Build frontend
cd ui && pnpm build

# Build backend
go build -ldflags="-s -w" -trimpath -o CodeKanban
```

**Build Artifacts**:
- Frontend: `ui/dist/` → `static/` (After moving to this directory, building the backend will automatically store it in the executable, achieving single-file launch)
- Backend: `CodeKanban.exe` (Windows) or `CodeKanban` (Linux/macOS)

### Access Application

**Development Environment**:
- Frontend Dev Server: `http://localhost:5173`
- Backend API: `http://localhost:3005`

**Production Environment**:
Run the built executable and access `http://localhost:3007`

## About

We are in an era that is both rapidly changing and extremely divided, and our work is the same.

This tool has genuinely improved my efficiency, but perhaps the improvement in efficiency will also reduce work demand, and the development of AI will eliminate this industry.

Anyway, I hope everyone enjoys using it.

If it helps you, you can give it a star or sponsor me a little.

### Possible Future Features
- Mobile support
- Code cleanup: For example, the frontend's src/api should all be auto-generated
- Completion reminder feature, such as playing a sound after AI is done to tell you it's finished.
- Idle terminal list / Terminal list awaiting interaction.
