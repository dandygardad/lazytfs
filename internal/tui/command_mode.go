package tui

import (
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// commands lists the command mode commands for autocomplete and the help shown on unknown commands.
var commands = []struct {
	name    string
	args    string
	aliases string
	desc    string
}{
	{"quit", "", "q", "quit lazytfs"},
	{"refresh", "", "r", "reload status, files and history"},
	{"get", "", "g", "get latest (pick files/folders)"},
	{"checkin", "", "ci", "checkin staged files"},
	{"conflicts", "", "c", "check and resolve conflicts"},
	{"ignored", "", "ig", "manage ignored paths"},
	{"changeset", "<id>", "cs", "open a changeset with its files and diffs"},
	{"author", "<name>", "au", "show changesets by author (empty = everyone)"},
	{"help", "", "h", "show keybindings"},
}

func (l *Layout) setupCommandMode() {
	l.CommandInput = tview.NewInputField().SetLabel(":").SetFieldWidth(0)

	l.CommandInput.SetAutocompleteFunc(func(text string) []string {
		if text == "" || strings.Contains(text, " ") {
			return nil
		}
		// An exact command or alias (e.g. "c") runs as typed, otherwise Enter would
		// pick the first suggestion ("checkin") instead of "conflicts".
		for _, c := range commands {
			if text == c.name || text == c.aliases {
				return nil
			}
		}
		var entries []string
		for _, c := range commands {
			if strings.HasPrefix(c.name, text) {
				if c.args != "" {
					entries = append(entries, c.name+" ") // leave room to type the argument
				} else {
					entries = append(entries, c.name)
				}
			}
		}
		return entries
	})
	l.CommandInput.SetAutocompletedFunc(func(text string, index int, source int) bool {
		l.CommandInput.SetText(text)
		// Enter on a command without arguments runs it right away
		if source == tview.AutocompletedEnter && !strings.HasSuffix(text, " ") {
			l.runCommand(text)
		}
		return true
	})

	l.CommandInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			l.runCommand(l.CommandInput.GetText())
		} else {
			l.closeCommandMode()
		}
	})

	l.BottomPages.AddPage("command", l.CommandInput, true, false)
}

func (l *Layout) openCommandMode() {
	l.CommandInput.SetText("")
	l.BottomPages.SwitchToPage("command")
	l.App.tviewApp.SetFocus(l.CommandInput)
}

func (l *Layout) closeCommandMode() {
	l.CommandInput.SetText("")
	l.BottomPages.SwitchToPage("bar")
	l.App.tviewApp.SetFocus(l.panels[l.currentPanel])
}

// runCommand executes a command typed after ":", e.g. "changeset 62826".
func (l *Layout) runCommand(text string) {
	l.closeCommandMode()
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return
	}
	arg := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), fields[0]))

	switch fields[0] {
	case "q", "q!", "qa", "quit", "wq", "x":
		l.App.tviewApp.Stop()
	case "r", "refresh":
		l.RefreshAll()
	case "g", "get":
		l.openGetLatest()
	case "ci", "checkin":
		l.showCheckinFlow()
	case "c", "conflicts":
		l.checkAndShowConflicts(".")
	case "ig", "ignored":
		l.showIgnoreList()
	case "h", "help":
		l.showHelp()
	case "cs", "changeset":
		changesetID := changesetIDFromText(arg)
		if changesetID == "" {
			l.MainView.SetTitle(" Command ")
			l.setMainText("Usage: :changeset <id>, e.g. :changeset 62826")
			return
		}
		l.showChangeset(changesetID)
	case "au", "author":
		l.searchHistory(arg)
		l.setFocus(3)
	default:
		// ":1" to ":4" jump to the left panels
		if n, err := strconv.Atoi(fields[0]); err == nil && n >= 1 && n <= 4 {
			l.setFocus(n - 1)
			return
		}
		var b strings.Builder
		b.WriteString("[red]Unknown command:[-] " + tview.Escape(text) + "\n\n[::b]Commands[::-]\n")
		for _, c := range commands {
			usage := ":" + c.name
			if c.args != "" {
				usage += " " + c.args
			}
			b.WriteString("  [yellow]" + tview.Escape(usage) + "[-]  [gray](:" + c.aliases + ")[-]  " + c.desc + "\n")
		}
		b.WriteString("  [yellow]:1 - :4[-]  jump to a left panel\n")
		l.MainView.SetTitle(" Command ")
		l.setMainText(b.String())
	}
}
