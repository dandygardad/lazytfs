package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/rivo/tview"
)

// renderStatusView shows a workspace dashboard in the Main View.
func (l *Layout) renderStatusView() {
	l.mu.Lock()
	defer l.mu.Unlock()

	var b strings.Builder
	section := func(title string) {
		b.WriteString("\n [::b]" + title + "[::-]\n [gray]" + strings.Repeat("─", 60) + "[-]\n")
	}
	row := func(label, value string) {
		fmt.Fprintf(&b, "  [gray]%-11s[-] %s\n", label, value)
	}
	key := func(k string) string {
		return "[yellow]" + tview.Escape("["+k+"]") + "[-]"
	}

	// Workspace + mappings
	section("Workspace")
	type mapping struct{ server, local string }
	var mappings []mapping
	var otherLines []string
	switch {
	case l.workspaceData == nil:
		b.WriteString("  [gray]Loading...[-]\n")
	case l.workspaceData[0] == "Error:":
		b.WriteString("  [red]" + tview.Escape(strings.Join(l.workspaceData, " ")) + "[-]\n")
	default:
		for _, line := range l.workspaceData {
			switch {
			case strings.HasPrefix(line, "Workspace"):
				_, v, _ := strings.Cut(line, ":")
				v = strings.TrimSpace(v)
				name, owner, _ := strings.Cut(v, " (")
				row("Name", "[green::b]"+tview.Escape(name)+"[-::-]")
				if owner != "" {
					row("Owner", tview.Escape(strings.TrimSuffix(owner, ")")))
				}
			case strings.HasPrefix(line, "Collection:"):
				row("Collection", "[blue]"+tview.Escape(strings.TrimSpace(strings.TrimPrefix(line, "Collection:")))+"[-]  "+key("o")+" [gray]open in browser[-]")
			case strings.Contains(line, "$/"):
				server, local, ok := strings.Cut(line, ": ")
				if ok {
					mappings = append(mappings, mapping{server, local})
				} else {
					otherLines = append(otherLines, line) // e.g. cloaked folders
				}
			}
		}
		row("Folder", tview.Escape(l.cwd))
	}

	if len(mappings) > 0 || len(otherLines) > 0 {
		section("Mappings")
		// The current mapping is the one with the longest local path containing the current folder
		current := -1
		lowerCwd := strings.ToLower(l.cwd)
		for i, m := range mappings {
			lowerLocal := strings.ToLower(m.local)
			if (lowerCwd == lowerLocal || strings.HasPrefix(lowerCwd, lowerLocal+`\`)) &&
				(current == -1 || len(m.local) > len(mappings[current].local)) {
				current = i
			}
		}
		for i, m := range mappings {
			if i == current {
				b.WriteString("  [green]●[-] [::b]" + tview.Escape(m.server) + "[::-] [gray]→[-] " + tview.Escape(m.local) + "  [green](you are here)[-]\n")
			} else {
				b.WriteString("  [gray]○ " + tview.Escape(m.server) + " → " + tview.Escape(m.local) + "[-]\n")
			}
		}
		for _, line := range otherLines {
			b.WriteString("  [gray]" + tview.Escape(line) + "[-]\n")
		}
	}

	// Pending changes
	section("Pending Changes")
	staged := 0
	switch {
	case l.filesData == nil:
		b.WriteString("  [gray]Loading...[-]\n")
	case l.filesData[0] == "Error:":
		b.WriteString("  [red]" + tview.Escape(strings.Join(l.filesData, " ")) + "[-]\n")
	default:
		counts := map[string]int{}
		total := 0
		for _, raw := range l.filesData {
			if !strings.Contains(raw, `:\`) {
				continue
			}
			total++
			if l.stagedFiles[raw] {
				staged++
			}
			_, status := parseTfStatusLine(raw)
			kind := strings.ToLower(strings.Trim(strings.Fields(status + " other")[0], ","))
			if kind != "edit" && kind != "add" && kind != "delete" {
				kind = "other"
			}
			counts[kind]++
		}

		if total == 0 {
			b.WriteString("  [green]✓ No pending changes, workspace is clean[-]\n")
			break
		}

		row("Total", "[::b]"+strconv.Itoa(total)+"[::-] changes")

		// Proportion bar of change types
		kinds := []struct{ name, color string }{{"edit", "yellow"}, {"add", "green"}, {"delete", "red"}, {"other", "gray"}}
		const barWidth = 40
		bar := ""
		legend := ""
		for _, k := range kinds {
			c := counts[k.name]
			if c == 0 {
				continue
			}
			n := max(c*barWidth/total, 1)
			bar += "[" + k.color + "]" + strings.Repeat("█", n) + "[-]"
			legend += "[" + k.color + "]■[-] " + k.name + " " + strconv.Itoa(c) + "   "
		}
		b.WriteString("  " + bar + "\n  " + legend + "\n")

		unstaged := total - staged - l.ignoredCount
		b.WriteString("  [green]" + strconv.Itoa(staged) + "[-] staged   [yellow]" + strconv.Itoa(unstaged) + "[-] unstaged   [gray]" + strconv.Itoa(l.ignoredCount) + " ignored[-]\n")
	}

	// Recent changesets
	section("Recent Changesets")
	switch {
	case l.historyData == nil:
		b.WriteString("  [gray]Loading...[-]\n")
	case l.historyData[0] == "Error:" || l.historyData[0] == "(None)":
		b.WriteString("  [gray]" + tview.Escape(strings.Join(l.historyData, " ")) + "[-]\n")
	default:
		for i, line := range l.historyData {
			if i == 5 {
				break
			}
			id, rest, _ := strings.Cut(line, " ")
			b.WriteString("  [yellow]" + tview.Escape(id) + "[-] " + tview.Escape(strings.TrimSpace(rest)) + "\n")
		}
		b.WriteString("  [gray]→ press " + tview.Escape("[4]") + " to browse all changesets[-]\n")
	}

	// Quick actions
	section("Quick Actions")
	checkin := "Checkin"
	if staged > 0 {
		checkin += " [green](" + strconv.Itoa(staged) + " staged)[-]"
	}
	b.WriteString("  " + key("g") + " Get latest    " + key("c") + " Conflicts    " + key("C") + " " + checkin + "\n")
	b.WriteString("  " + key("r") + " Refresh       " + key("I") + " Ignored paths (" + strconv.Itoa(len(l.ignoredPaths)) + ")    " + key("?") + " Help\n")

	l.MainView.SetTitle(" Status ")
	l.setMainText(b.String())
	l.MainView.ScrollToBeginning()
}

// collectionURL returns the TFS collection URL from the workspace info, or "".
func (l *Layout) collectionURL() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, line := range l.workspaceData {
		if strings.HasPrefix(line, "Collection:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Collection:"))
		}
	}
	return ""
}

func openBrowser(url string) {
	switch runtime.GOOS {
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		exec.Command("open", url).Start()
	case "linux":
		exec.Command("xdg-open", url).Start()
	}
}
