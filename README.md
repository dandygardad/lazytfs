# lazytfs
*simple ui for tfs*

A terminal UI for Team Foundation Server (TFS) version control inspired by `lazygit`. Built with Go and `tview`.

## Features
- **4-Panel Layout:** View Workspace Status, Unstaged Files, Staged Files, and Changesets (History).
- **Interactive Diff Viewing:** Select a file to view a colorized diff in the main view.
- **Stage/Unstage:** Easily stage and unstage files using the `[Space]` bar.
- **Context-Aware Search (`/`):** Filter files by name, or filter changesets by author name.
- **Command Log:** Real-time visibility into the underlying `tf` commands being executed in the background.
- **Fast Navigation:** Jump between panels instantly using hotkeys `[1-6]`.
- **Help Modal:** Press `[?]` to view a list of all available global and panel-specific hotkeys.

## Prerequisites
- **TFS CLI (`tf.exe`)**: Ensure `tf.exe` is installed and available in your system's `PATH`.

## Installation

```bash
git clone https://github.com/dandygardad/lazytfs.git
cd lazytfs
go build -o lazytfs.exe ./cmd
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
- `[?]` - Show help menu
- `[1-4]` - Jump to left panels (Status, Unstaged, Staged, Changesets)
- `[5]` - Jump to Main View (useful for scrolling long diffs)
- `[6]` - Jump to Command Log

**Panel Specific (Files / History):**
- `[Space]` - Stage/Unstage file
- `[/]` - Search files or authors
- `[Enter]` - View diff / Select

---
*Built by [dg](https://dandygarda.com)*
