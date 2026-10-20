# lazytfs
*simple ui for tfs*

A terminal UI for Team Foundation Server (TFS) version control inspired by `lazygit`. Built with Go and `tview`.

## Features
- **4-Panel Layout:** View Workspace Status, Unstaged Files, Staged Files, and Changesets (History).
- **Side-by-Side Diff:** Press `[Enter]` on a file to see server (left) vs local (right) with line numbers and highlighted removed/added lines. Press `[5]` to focus it, then scroll with `[↑/↓]`/`[PgUp/PgDn]`, scroll sideways with `[←/→]`, and jump between changes with `[n/p]`. Mouse wheel works too.
- **Tree View Staging:** Unstaged and Staged files are shown as a folder tree (single-folder chains are compacted, e.g. `src\app\models`). Stage a single file or a whole folder with `[Space]`, collapse/expand folders with `[←/→]`.
- **Clean Status UI**: File statuses are colored and formatted (e.g., `File.cs (Edit)`, `File.cs (New)`).
- **Ignore Paths (`i` / `I`):** Hide files or folders you never check in (e.g. `bin`, `obj`) from Unstaged. The list is saved in `%AppData%\lazytfs\ignore.txt` and can be managed with `[I]`.
- **Unified Checkin Flow (`C`):** A beautiful 3-step modal UI to preview staged files, write your checkin message, and confirm the checkin.
- **Get Latest (`g`):** Browse a file/folder tree, check one or more files/folders with `[Space]` (`[→/←]` to expand/collapse), then press `[Enter]` to fetch the latest server version recursively.
- **Conflict Resolution (`c`):** Dedicated UI to handle conflicts per-file (Take Server vs. Keep Mine) seamlessly.
- **Changeset Explorer (`Enter` on a changeset):** See who/when/comment, the list of changed files (A/M/D/R), and a side-by-side diff of each file against the previous version. New/deleted files are shown in full.
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
- `[g]` - Get Latest (pick files/folders in a tree)
- `[c]` - Check and resolve conflicts
- `[C]` - Checkin staged files
- `[?]` - Show help menu
- `[1-4]` - Jump to left panels (Status, Unstaged, Staged, Changesets)
- `[5]` - Jump to Main View (useful for scrolling long diffs)
- `[6]` - Jump to Command Log
- `[I]` - Manage ignored paths

**Panel Specific (Files / History):**
- `[Space]` - Stage/Unstage file (or every file under a folder)
- `[←/→]` - Collapse/Expand folder (`←` on a file jumps to its folder)
- `[i]` - Ignore file/folder in Unstaged
- `[/]` - Search files or authors
- `[Enter]` - View diff / Toggle folder / Open changeset files & diff
- `[o]` - Open selected changeset in browser (Changesets panel)
- `[Ctrl+S]` - Submit checkin message (Enter adds a new line)

---
*Built by [dg](https://dandygarda.com)*
