package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// editorChoice is one editor offered when pressing "e" on a file.
type editorChoice struct {
	name      string
	cmd       []string // command and args, the file path is appended
	terminal  bool     // runs inside this terminal, so lazytfs is suspended until it exits
	lineStyle string   // how to open at a line: "plus" (+N file), "colon" (file:N), "goto" (-g file:N), "npp" (-nN file), "" (not supported)
}

// knownEditors are offered in the picker when found in PATH.
var knownEditors = []editorChoice{
	{"Neovim", []string{"nvim"}, true, "plus"},
	{"Vim", []string{"vim"}, true, "plus"},
	{"Helix", []string{"hx"}, true, "colon"},
	{"Micro", []string{"micro"}, true, "plus"},
	{"Nano", []string{"nano"}, true, "plus"},
	{"VS Code", []string{"code"}, false, "goto"},
	{"Cursor", []string{"cursor"}, false, "goto"},
	{"Sublime Text", []string{"subl"}, false, "colon"},
	{"Notepad++", []string{"notepad++"}, false, "npp"},
	{"Notepad", []string{"notepad"}, false, ""},
}

func (l *Layout) setupEditorPicker() {
	l.EditorList = tview.NewList().ShowSecondaryText(false).SetHighlightFullLine(true)
	l.EditorList.SetBorder(true).SetTitleColor(tcell.ColorYellow)
	l.EditorList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			l.Pages.HidePage("editor_page")
			l.App.tviewApp.SetFocus(l.editorReturnFocus)
			return nil
		}
		return event
	})

	inner := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(l.EditorList, 0, 1, true).
		AddItem(tview.NewTextView().SetText(" [enter/1-9] open | [esc] cancel "), 1, 0, false)

	l.EditorPage = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(nil, 0, 1, false).
			AddItem(inner, 60, 1, true).
			AddItem(nil, 0, 1, false), 16, 1, true).
		AddItem(nil, 0, 1, false)
}

// showEditorPicker lists the editors installed on this machine to open filename with,
// at the given line when line > 0 and the editor supports it.
func (l *Layout) showEditorPicker(filename string, line int) {
	var choices []editorChoice
	if env := strings.Fields(os.Getenv("EDITOR")); len(env) > 0 {
		lineStyle := ""
		for _, e := range knownEditors {
			if filepath.Base(env[0]) == e.cmd[0] || strings.TrimSuffix(filepath.Base(env[0]), ".exe") == e.cmd[0] {
				lineStyle = e.lineStyle // e.g. EDITOR=nvim opens at the line like Neovim does
			}
		}
		choices = append(choices, editorChoice{"EDITOR", env, true, lineStyle})
	}
	for _, e := range knownEditors {
		if _, err := exec.LookPath(e.cmd[0]); err == nil {
			choices = append(choices, e)
		}
	}
	if len(choices) == 0 {
		l.setMainText("No editor found in PATH. Install one (e.g. nvim, vim, code) or set the EDITOR environment variable.")
		return
	}

	l.EditorList.Clear()
	title := filepath.Base(filename)
	if line > 0 {
		title += ":" + strconv.Itoa(line)
	}
	l.EditorList.SetTitle(" Open " + title + " with ")
	current := 0
	for i, c := range choices {
		kind := "terminal"
		if !c.terminal {
			kind = "window"
		}
		var shortcut rune
		if i < 9 {
			shortcut = rune('1' + i)
		}
		l.EditorList.AddItem(c.name+"  [gray]"+tview.Escape(strings.Join(c.cmd, " "))+" · "+kind+"[-]", "", shortcut, func() {
			l.Pages.HidePage("editor_page")
			l.App.tviewApp.SetFocus(l.editorReturnFocus)
			l.lastEditor = c.name
			l.openInEditor(c, filename, line)
		})
		if c.name == l.lastEditor {
			current = i
		}
	}
	l.EditorList.SetCurrentItem(current)
	l.editorReturnFocus = l.App.tviewApp.GetFocus() // back to the files tree or the diff afterwards
	l.Pages.ShowPage("editor_page")
	l.App.tviewApp.SetFocus(l.EditorList)
}

func (l *Layout) openInEditor(c editorChoice, filename string, line int) {
	args := append([]string{}, c.cmd[1:]...)
	n := strconv.Itoa(line)
	switch {
	case line <= 0:
		args = append(args, filename)
	case c.lineStyle == "plus":
		args = append(args, "+"+n, filename)
	case c.lineStyle == "colon":
		args = append(args, filename+":"+n)
	case c.lineStyle == "goto":
		args = append(args, "-g", filename+":"+n)
	case c.lineStyle == "npp":
		args = append(args, "-n"+n, filename)
	default:
		args = append(args, filename)
	}
	cmd := exec.Command(c.cmd[0], args...)

	// GUI editors open their own window, lazytfs keeps running
	if !c.terminal {
		if err := cmd.Start(); err != nil {
			l.setMainText("Failed to open " + c.name + ":\n" + err.Error())
			return
		}
		go cmd.Wait() // release the process when the editor closes
		l.setMainText("Opened " + filename + " in " + c.name)
		return
	}

	var runErr error
	l.App.tviewApp.Suspend(func() {
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		runErr = cmd.Run()
	})
	if runErr != nil {
		l.setMainText("Failed to open " + c.name + " (" + strings.Join(c.cmd, " ") + "):\n" + runErr.Error())
	}
}
