package tui

import (
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// changesetItem is one changed file in a changeset.
type changesetItem struct {
	change     string // e.g. "edit", "add", "merge, edit"
	serverPath string
}

// changesetDiff is a loaded diff of one changeset item, cached so revisiting a file is instant.
type changesetDiff struct {
	rows           []diffRow
	added, removed int
}

func (l *Layout) setupChangesetUI() {
	l.ChangesetHeader = tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
	l.ChangesetHeader.SetBorder(true).SetTitle(" Changeset ").SetTitleColor(tcell.ColorYellow)

	l.ChangesetFiles = tview.NewList().ShowSecondaryText(false).SetHighlightFullLine(true)
	l.ChangesetFiles.SetBorder(true).SetTitle(" Files ")

	l.ChangesetDiff = NewDiffView()

	hint := tview.NewTextView().SetText(" [↑/↓] pick file | [enter/tab] switch files/diff | [n/p] next/prev change | [w] whitespace on/off | [←/→] scroll sideways | [o] open in browser | [esc] back ")

	body := tview.NewFlex().
		AddItem(l.ChangesetFiles, 0, 3, true).
		AddItem(l.ChangesetDiff, 0, 7, false)

	l.ChangesetPage = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(l.ChangesetHeader, 6, 0, false).
		AddItem(body, 0, 1, true).
		AddItem(hint, 1, 0, false)

	openInBrowser := func() {
		collectionURL := l.collectionURL()
		if collectionURL != "" {
			openBrowser(strings.TrimRight(collectionURL, "/") + "/_versionControl/changeset/" + strconv.Itoa(l.changesetID))
		}
	}

	l.ChangesetFiles.SetChangedFunc(func(i int, mainText string, secondaryText string, shortcut rune) {
		l.loadChangesetDiff(i)
	})
	l.ChangesetFiles.SetSelectedFunc(func(i int, mainText string, secondaryText string, shortcut rune) {
		l.App.tviewApp.SetFocus(l.ChangesetDiff)
	})
	l.ChangesetFiles.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyEsc:
			l.Pages.HidePage("changeset_page")
			l.App.tviewApp.SetFocus(l.HistoryList)
			return nil
		case event.Key() == tcell.KeyTab:
			l.App.tviewApp.SetFocus(l.ChangesetDiff)
			return nil
		case event.Key() == tcell.KeyRune && event.Rune() == 'o':
			openInBrowser()
			return nil
		}
		return event
	})
	l.ChangesetDiff.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyRune && event.Rune() == 'w':
			// Toggle whitespace changes and reload, cached diffs used the old setting
			l.ChangesetDiff.IgnoreSpace = !l.ChangesetDiff.IgnoreSpace
			l.changesetDiffCache = make(map[int]changesetDiff)
			l.loadChangesetDiff(l.ChangesetFiles.GetCurrentItem())
			return nil
		case event.Key() == tcell.KeyEsc || event.Key() == tcell.KeyTab:
			l.App.tviewApp.SetFocus(l.ChangesetFiles)
			return nil
		case event.Key() == tcell.KeyRune && event.Rune() == 'o':
			openInBrowser()
			return nil
		}
		return event
	})
}

// showChangeset opens the changeset page: info on top, changed files on the left
// and the side-by-side diff of the selected file on the right.
func (l *Layout) showChangeset(changesetID string) {
	id, _ := strconv.Atoi(changesetID)
	l.changesetID = id
	l.changesetItems = nil
	l.changesetDiffCache = make(map[int]changesetDiff)
	l.ChangesetHeader.SetText("Loading changeset " + changesetID + "...")
	l.ChangesetFiles.Clear()
	l.ChangesetFiles.SetTitle(" Files ")
	l.ChangesetDiff.SetDiff("", nil, 0, 0)
	l.Pages.ShowPage("changeset_page")
	l.App.tviewApp.SetFocus(l.ChangesetFiles)

	go func() {
		out, err := l.App.tfsClient.GetChangesetDetail(changesetID)
		l.App.tviewApp.QueueUpdateDraw(func() {
			if l.changesetID != id {
				return // another changeset was opened meanwhile
			}
			if err != nil {
				l.ChangesetHeader.SetText("[red]Error getting changeset detail:[-]\n" + tview.Escape(err.Error()+"\n"+out))
				return
			}

			var user, date string
			var comment []string
			var items []changesetItem
			section := ""
			for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
				trimmed := strings.TrimSpace(line)
				switch {
				case strings.HasPrefix(line, "User:"):
					user = strings.TrimSpace(strings.TrimPrefix(line, "User:"))
				case strings.HasPrefix(line, "Date:"):
					date = strings.TrimSpace(strings.TrimPrefix(line, "Date:"))
				case strings.HasPrefix(line, "Comment:"):
					section = "comment"
				case strings.HasPrefix(line, "Items:"):
					section = "items"
				case trimmed != "" && !strings.HasPrefix(line, " "):
					section = "" // other sections like "Check-in Notes:"
				case section == "comment" && trimmed != "":
					comment = append(comment, trimmed)
				case section == "items" && trimmed != "":
					if i := strings.Index(trimmed, "$/"); i != -1 {
						items = append(items, changesetItem{change: strings.TrimSpace(trimmed[:i]), serverPath: trimmed[i:]})
					}
				}
			}

			l.ChangesetHeader.SetText("[yellow::b]Changeset " + changesetID + "[-::-]   [gray]by[-] [::b]" + tview.Escape(user) + "[::-]   [gray]on[-] " + tview.Escape(date) +
				"\n\n" + tview.Escape(strings.Join(comment, "\n")))

			l.changesetItems = items
			l.ChangesetFiles.SetTitle(" Files (" + strconv.Itoa(len(items)) + ") ")
			for _, item := range items {
				dir, name := path.Split(item.serverPath)
				l.ChangesetFiles.AddItem(changeBadge(item.change)+" "+tview.Escape(name)+"  [gray]"+tview.Escape(strings.TrimSuffix(dir, "/"))+"[-]", "", 0, nil)
			}
			if len(items) > 0 {
				l.ChangesetFiles.SetCurrentItem(0)
				l.loadChangesetDiff(0)
			}
		})
	}()
}

// loadChangesetDiff shows the diff of changeset item i, from cache or by asking tf.
func (l *Layout) loadChangesetDiff(i int) {
	if i < 0 || i >= len(l.changesetItems) {
		return
	}
	item := l.changesetItems[i]
	name := path.Base(item.serverPath)
	if c, ok := l.changesetDiffCache[i]; ok {
		l.ChangesetDiff.SetDiff(name, c.rows, c.added, c.removed)
		return
	}

	l.ChangesetDiff.SetDiff(name, textRows("Loading diff..."), 0, 0)
	seq := l.changesetSeq.Add(1)
	id := l.changesetID
	ignoreSpace := l.ChangesetDiff.IgnoreSpace
	go func() {
		// Wait a bit so moving quickly through the list doesn't start a tf call per file
		time.Sleep(250 * time.Millisecond)
		if l.changesetSeq.Load() != seq {
			return
		}

		var c changesetDiff
		change := strings.ToLower(item.change)
		switch {
		case strings.Contains(change, "undelete") || strings.Contains(change, "add") || strings.Contains(change, "branch"):
			// No previous version, show the whole new file
			out, err := l.App.tfsClient.ViewFile(item.serverPath, id)
			if err != nil {
				c.rows = textRows("Error viewing file:\n" + err.Error() + "\n" + out)
			} else {
				c.rows, c.added = wholeFileRows("New file in this changeset", out, true)
			}
		case strings.Contains(change, "delete"):
			// No new version, show the whole file as it was before
			out, err := l.App.tfsClient.ViewFile(item.serverPath, id-1)
			if err != nil {
				c.rows = textRows("Error viewing file:\n" + err.Error() + "\n" + out)
			} else {
				c.rows, c.removed = wholeFileRows("Deleted in this changeset", out, false)
			}
		default:
			out, err := l.App.tfsClient.GetChangesetFileDiff(item.serverPath, id, ignoreSpace)
			if err != nil {
				c.rows = textRows("Error getting diff:\n" + err.Error() + "\n" + out)
			} else if c.rows, c.added, c.removed = parseUnifiedDiff(out); len(c.rows) == 0 {
				c.rows = textRows(out) // e.g. binary files
			}
		}

		l.App.tviewApp.QueueUpdateDraw(func() {
			if l.changesetID != id {
				return
			}
			l.changesetDiffCache[i] = c
			if l.changesetSeq.Load() == seq {
				l.ChangesetDiff.SetDiff(name, c.rows, c.added, c.removed)
			}
		})
	}()
}

// textRows shows plain text (messages, errors, non-diff output) in a DiffView.
func textRows(text string) []diffRow {
	var rows []diffRow
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		rows = append(rows, diffRow{kind: 'h', oldText: strings.ReplaceAll(line, "\t", "    ")})
	}
	return rows
}

// wholeFileRows shows a whole file as added (right side) or removed (left side).
func wholeFileRows(title, content string, added bool) ([]diffRow, int) {
	rows := []diffRow{{kind: 'h', oldText: title}}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for i, line := range lines {
		line = strings.ReplaceAll(line, "\t", "    ")
		if added {
			rows = append(rows, diffRow{kind: 'c', newNo: i + 1, newText: line, added: true})
		} else {
			rows = append(rows, diffRow{kind: 'c', oldNo: i + 1, oldText: line, removed: true})
		}
	}
	return rows, len(lines)
}
