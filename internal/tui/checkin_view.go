package tui

import (
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var checkedInRe = regexp.MustCompile(`Changeset #(\d+)`)

// changeBadge returns a colored one letter badge for a tf change type like "edit" or "add, encoding".
func changeBadge(change string) string {
	change = strings.ToLower(change)
	switch {
	case strings.Contains(change, "undelete") || strings.Contains(change, "add") || strings.Contains(change, "branch"):
		return "[green::b]A[-::-]"
	case strings.Contains(change, "delete"):
		return "[red::b]D[-::-]"
	case strings.Contains(change, "rename"):
		return "[blue::b]R[-::-]"
	}
	return "[yellow::b]M[-::-]"
}

func (l *Layout) setupCheckinUI() {
	l.CheckinSummary = tview.NewTextView().SetDynamicColors(true)

	l.CheckinPreview = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	l.CheckinPreview.SetBorder(true).SetTitle(" Files ").SetTitleAlign(tview.AlignLeft)

	l.CheckinInput = tview.NewTextArea().SetPlaceholder("Describe your change... (Enter = new line)")
	l.CheckinInput.SetBorder(true).SetTitle(" Message ").SetTitleAlign(tview.AlignLeft)

	l.CheckinCounter = tview.NewTextView().SetDynamicColors(true)
	l.CheckinInput.SetChangedFunc(func() {
		text := l.CheckinInput.GetText()
		lines := 0
		if strings.TrimSpace(text) != "" {
			lines = strings.Count(text, "\n") + 1
		}
		l.CheckinCounter.SetText(" [gray]" + strconv.Itoa(lines) + " lines · " + strconv.Itoa(len([]rune(text))) + " chars[-]")
	})

	hint := tview.NewTextView().SetDynamicColors(true).
		SetText(" [yellow]ctrl+s[-] review & checkin   [yellow]tab[-] switch files/message   [yellow]esc[-] cancel")

	inner := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(l.CheckinSummary, 3, 0, false).
		AddItem(l.CheckinPreview, 0, 1, false).
		AddItem(l.CheckinInput, 8, 0, true).
		AddItem(l.CheckinCounter, 1, 0, false).
		AddItem(hint, 1, 0, false)
	inner.SetBorder(true).SetTitle(" ✔ Checkin ").SetTitleColor(tcell.ColorGreen)

	// Sized relative to the terminal so long paths and messages have room
	l.CheckinPage = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(inner, 0, 8, true).
			AddItem(nil, 0, 1, false), 0, 8, true).
		AddItem(nil, 0, 1, false)

	closePage := func() {
		l.Pages.HidePage("checkin_page")
		l.App.tviewApp.SetFocus(l.panels[l.currentPanel])
	}

	// Enter inserts a new line (default TextArea behavior), Ctrl+S goes to the review
	l.CheckinInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyCtrlS:
			msg := strings.TrimSpace(l.CheckinInput.GetText())
			if msg == "" {
				l.CheckinCounter.SetText(" [red]Please write a checkin message first[-]")
				return nil
			}
			firstLine, _, _ := strings.Cut(msg, "\n")
			if len([]rune(firstLine)) > 50 {
				firstLine = string([]rune(firstLine)[:50]) + "…"
			}
			l.CheckinConfirm1.SetText("Check in " + strconv.Itoa(len(l.checkinFiles)) + " file(s) with message:\n\n\"" + firstLine + "\"")
			l.CheckinConfirm1.GetForm().SetFocus(1) // default to Cancel, so a stray Enter doesn't continue
			l.Pages.ShowPage("checkin_confirm1")
			l.App.tviewApp.SetFocus(l.CheckinConfirm1)
			return nil
		case tcell.KeyTab:
			l.App.tviewApp.SetFocus(l.CheckinPreview)
			return nil
		case tcell.KeyEsc:
			closePage()
			return nil
		}
		return event
	})
	l.CheckinPreview.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab:
			l.App.tviewApp.SetFocus(l.CheckinInput)
			return nil
		case tcell.KeyEsc:
			closePage()
			return nil
		}
		return event
	})

	// Confirm 1: summary
	l.CheckinConfirm1 = NewCustomModal("", []string{"OK", "Cancel"}, func(buttonIndex int, buttonLabel string) {
		l.Pages.HidePage("checkin_confirm1")
		if buttonLabel == "OK" {
			l.checkinConfirm2Form.SetFocus(1) // default to Cancel
			l.Pages.ShowPage("checkin_confirm2")
			l.App.tviewApp.SetFocus(l.CheckinConfirm2)
		} else {
			l.App.tviewApp.SetFocus(l.CheckinInput)
		}
	})

	// Confirm 2: final, with colored buttons
	form := tview.NewForm().
		AddButton("OK, check in", func() {
			l.Pages.HidePage("checkin_confirm2")
			l.Pages.HidePage("checkin_page")
			l.runCheckin()
		}).
		AddButton("Cancel", func() {
			l.Pages.HidePage("checkin_confirm2")
			l.App.tviewApp.SetFocus(l.CheckinInput)
		})
	form.SetButtonsAlign(tview.AlignCenter)
	l.checkinConfirm2Form = form

	for i := 0; i < form.GetButtonCount(); i++ {
		btn := form.GetButton(i)
		originalLabel := btn.GetLabel()
		btn.SetFocusFunc(func() {
			btn.SetLabel("> " + originalLabel + " <")
		})
		btn.SetBlurFunc(func() {
			btn.SetLabel(originalLabel)
		})
	}
	buttonYes := form.GetButton(0)
	buttonYes.SetBackgroundColor(tcell.ColorGreen)
	buttonYes.SetLabelColor(tcell.ColorWhite)
	buttonNo := form.GetButton(1)
	buttonNo.SetBackgroundColor(tcell.ColorRed)
	buttonNo.SetLabelColor(tcell.ColorWhite)

	textView := tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignCenter).
		SetText("\n[::b]This can't be undone.[::-]\nCheck in to TFS now?\n")

	innerConfirm := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(textView, 4, 1, false).
		AddItem(form, 3, 1, true)
	innerConfirm.SetBorder(true).SetTitle(" Final Confirmation ").SetTitleColor(tcell.ColorRed)

	l.CheckinConfirm2 = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(nil, 0, 1, false).
			AddItem(innerConfirm, 50, 1, true).
			AddItem(nil, 0, 1, false), 9, 1, true).
		AddItem(nil, 0, 1, false)

	// Escape goes back, left/right moves between buttons
	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			l.Pages.HidePage("checkin_confirm2")
			l.App.tviewApp.SetFocus(l.CheckinInput)
			return nil
		case tcell.KeyLeft:
			return tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone)
		case tcell.KeyRight:
			return tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)
		}
		return event
	})
}

func (l *Layout) showCheckinFlow() {
	l.mu.Lock()
	var staged []string
	for file, isStaged := range l.stagedFiles {
		if isStaged {
			staged = append(staged, file)
		}
	}
	l.mu.Unlock()

	if len(staged) == 0 {
		l.MainView.SetTitle(" Checkin ")
		l.setMainText("[yellow]No files staged for checkin.[-]\n\nStage files in the Unstaged panel with [::b]Space[::-] first.")
		return
	}

	type stagedFile struct{ rel, change, local string }
	var files []stagedFile
	counts := map[string]int{}
	for _, raw := range staged {
		local := extractFilePath(raw)
		rel := local
		if r, err := filepath.Rel(l.cwd, local); err == nil && !strings.HasPrefix(r, "..") {
			rel = r
		}
		_, change := parseTfStatusLine(raw)
		files = append(files, stagedFile{rel, change, local})
		counts[changeBadge(change)]++
	}
	sort.Slice(files, func(i, j int) bool { return strings.ToLower(files[i].rel) < strings.ToLower(files[j].rel) })

	l.checkinFiles = nil
	var b strings.Builder
	for _, f := range files {
		l.checkinFiles = append(l.checkinFiles, f.local)
		b.WriteString(" " + changeBadge(f.change) + "  " + tview.Escape(f.rel) + "  [gray]" + tview.Escape(f.change) + "[-]\n")
	}
	l.CheckinPreview.SetText(b.String()).ScrollToBeginning()
	l.CheckinPreview.SetTitle(" Files (" + strconv.Itoa(len(files)) + ") ")

	summary := " [::b]" + strconv.Itoa(len(files)) + " file(s) to check in[::-]   "
	for _, badge := range []string{changeBadge("edit"), changeBadge("add"), changeBadge("delete"), changeBadge("rename")} {
		if counts[badge] > 0 {
			summary += badge + " " + strconv.Itoa(counts[badge]) + "   "
		}
	}
	summary += "\n [gray]from " + tview.Escape(l.cwd) + "[-]"
	l.CheckinSummary.SetText(summary)

	l.Pages.ShowPage("checkin_page")
	l.App.tviewApp.SetFocus(l.CheckinInput)
}

// runCheckin checks in the staged files and shows the result in the Main View.
func (l *Layout) runCheckin() {
	var filesToCkin []string
	l.mu.Lock()
	for f, staged := range l.stagedFiles {
		if staged {
			if extractedPath := extractFilePath(f); extractedPath != "" {
				filesToCkin = append(filesToCkin, extractedPath)
			}
		}
	}
	l.mu.Unlock()

	msg := l.CheckinInput.GetText()

	// Without file args, tf checkin would check in ALL pending changes
	if len(filesToCkin) == 0 {
		l.MainView.SetTitle(" Checkin Error ")
		l.setMainText("[red]No valid file paths found in staged files. Checkin cancelled.[-]")
		l.App.tviewApp.SetFocus(l.panels[l.currentPanel])
		return
	}

	l.LoadingModal.SetText("Checking in " + strconv.Itoa(len(filesToCkin)) + " file(s)...")
	l.Pages.ShowPage("loading")
	l.App.tviewApp.SetFocus(l.LoadingModal)

	go func() {
		out, err := l.App.tfsClient.Checkin(filesToCkin, msg)
		l.App.tviewApp.QueueUpdateDraw(func() {
			l.Pages.HidePage("loading")
			l.App.tviewApp.SetFocus(l.panels[l.currentPanel])

			if err != nil {
				// Keep the message and staged files so the user can fix the issue and retry
				l.MainView.SetTitle(" Checkin Failed ")
				l.setMainText("[red::b]✗ Checkin failed[-::-]\n\n" + tview.Escape(err.Error()+"\n"+out) +
					"\n\n[gray]Your message and staged files are kept. Fix the issue and press [::b]C[::-] to try again.[-]")
				return
			}

			var b strings.Builder
			if m := checkedInRe.FindStringSubmatch(out); m != nil {
				b.WriteString("[green::b]✓ Checked in as changeset " + m[1] + "[-::-]\n\n")
			} else {
				b.WriteString("[green::b]✓ Checked in[-::-]\n\n")
			}
			b.WriteString("[gray]Message[-]\n")
			for _, line := range strings.Split(strings.TrimSpace(msg), "\n") {
				b.WriteString("  " + tview.Escape(line) + "\n")
			}
			b.WriteString("\n[gray]Files (" + strconv.Itoa(len(filesToCkin)) + ")[-]\n")
			for _, f := range filesToCkin {
				b.WriteString("  " + tview.Escape(f) + "\n")
			}
			b.WriteString("\n[gray]tf output[-]\n[gray]" + tview.Escape(out) + "[-]\n")
			if m := checkedInRe.FindStringSubmatch(out); m != nil {
				b.WriteString("\n[gray]Tip: type [::b]:cs " + m[1] + "[::-] to view this changeset.[-]\n")
			}
			l.MainView.SetTitle(" Checkin Success ")
			l.setMainText(b.String())
			l.MainView.ScrollToBeginning()

			l.CheckinInput.SetText("", false)
			l.mu.Lock()
			for k := range l.stagedFiles {
				delete(l.stagedFiles, k)
			}
			l.mu.Unlock()
			l.RefreshAll()
		})
	}()
}
