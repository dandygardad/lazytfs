# lazytfs
*simple ui for tfs*

A terminal UI for Team Foundation Server (TFS) version control inspired by `lazygit`. Built with Go and `tview`.

## Features
- **4-Panel Layout:** View Workspace Status, Unstaged Files, Staged Files, and Changesets (History).
- **Interactive Diff Viewing:** Select a file to view a colorized diff in the main view.
- **Smart Staging:** 
  - Stage and unstage individual files using the `[Space]` bar.
  - **Tree-like Folder Staging**: Press `[Space]` on a folder header (`📁 $/...`) to instantly stage or unstage all files under that folder.
- **Clean Status UI**: File statuses are elegantly formatted (e.g., `(Edit) - File.cs`, `(New) - File.cs`).
- **Unified Checkin Flow (`C`):** A beautiful 3-step modal UI to preview staged files, write your checkin message, and confirm the checkin.
- **Get Latest (`g`):** Open a directory tree to fetch the latest server versions recursively.
- **Conflict Resolution (`c`):** Dedicated UI to handle conflicts per-file (Take Server vs. Keep Mine) seamlessly.
- **Server-Side History Search (`/`):** Filter changesets by fetching directly from the TFS server based on author or keywords.
- **Command Log:** Real-time visibility into the underlying `tf` commands being executed in the background.
- **Fast Navigation:** Jump between panels instantly using hotkeys `[1-6]`.

## Prerequisites
- **TFS CLI (`tf.exe`)**: Ensure `tf.exe` is installed and available in your system's `PATH` (typically comes with Visual Studio).

## Installation

### Option 1: Download Release
You can download the latest pre-built `lazytfs.exe` directly from the [GitHub Releases](https://github.com/dandygardad/lazytfs/releases) page. The binaries are automatically built using GitHub Actions.

### Option 2: Build from Source
```bash
git clone https://github.com/dandygardad/lazytfs.git
cd lazytfs
go build -o lazytfs.exe ./cmd/main.go
```

## Usage
Run the executable inside your mapped TFS workspace folder:
```bash
./lazytfs.exe
```

### Hotkeys
**Global:**
- `[q]` - Quit
- `[r]` - Refresh data
- `[g]` - Get Latest (opens directory tree)
- `[c]` - Check and resolve conflicts
- `[C]` - Checkin staged files
- `[?]` - Show help menu
- `[1-4]` - Jump to left panels (Status, Unstaged, Staged, Changesets)
- `[5]` - Jump to Main View (useful for scrolling long diffs)
- `[6]` - Jump to Command Log

**Panel Specific (Files / History):**
- `[Space]` - Stage/Unstage file (or all files under a folder header)
- `[/]` - Search files or authors
- `[Enter]` - View diff / Select / Open Changeset URL

---
*Built by [dg](https://dandygarda.com)*
