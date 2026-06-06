# lazytfs
*simple ui for tfs*

A terminal UI for Team Foundation Server (TFS) version control inspired by `lazygit`. Built with Go and `tview`.

## Features
- **4-Panel Layout:** View Workspace Status, Unstaged Files, Staged Files, and Changesets (History).
- **Interactive Diff Viewing:** Select a file to view a colorized diff in the main view.
- **Smart Staging:** 
  - Stage and unstage individual files using the `[Space]` bar.
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

You can easily install `lazytfs` using `go install`. This requires you to have Go installed on your machine.

```bash
go install github.com/dandygardad/lazytfs@latest
```

This will download, compile, and install the `lazytfs` binary into your `$GOPATH/bin` folder. Make sure your Go bin directory is added to your system's `PATH`.

## Usage
Run the tool inside your mapped TFS workspace folder:
```bash
lazytfs
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
