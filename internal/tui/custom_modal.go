package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type CustomModal struct {
	*tview.Flex
	textView *tview.TextView
	form     *tview.Form
}

func NewCustomModal(text string, buttons []string, doneFunc func(buttonIndex int, buttonLabel string)) *CustomModal {
	m := &CustomModal{}
	m.textView = tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText("\n" + text + "\n")

	m.form = tview.NewForm()
	for i, btnLabel := range buttons {
		idx := i
		label := btnLabel
		m.form.AddButton(label, func() {
			if doneFunc != nil {
				doneFunc(idx, label)
			}
		})
	}
	m.form.SetButtonsAlign(tview.AlignCenter)

	for i := 0; i < m.form.GetButtonCount(); i++ {
		btn := m.form.GetButton(i)
		originalLabel := buttons[i]
		
		btn.SetFocusFunc(func() {
			btn.SetLabel("> " + originalLabel + " <")
		})
		btn.SetBlurFunc(func() {
			btn.SetLabel(originalLabel)
		})
	}

	innerFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(m.textView, 0, 1, false)
	
	if len(buttons) > 0 {
		innerFlex.AddItem(m.form, 3, 1, true)
	}
	
	innerFlex.SetBorder(true)

	m.Flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(nil, 0, 1, false).
			AddItem(innerFlex, 60, 1, true).
			AddItem(nil, 0, 1, false), 15, 1, true).
		AddItem(nil, 0, 1, false)

	if len(buttons) > 0 {
		m.form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Key() == tcell.KeyLeft {
				return tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone)
			} else if event.Key() == tcell.KeyRight {
				return tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)
			}
			return event
		})
	}

	return m
}

func (m *CustomModal) SetText(text string) *CustomModal {
	m.textView.SetText("\n" + text + "\n")
	return m
}

func (m *CustomModal) SetInputCapture(capture func(event *tcell.EventKey) *tcell.EventKey) *CustomModal {
	originalCapture := m.form.GetInputCapture()
	if m.form.GetButtonCount() == 0 {
		m.Flex.SetInputCapture(capture)
		return m
	}
	
	m.form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if capture != nil {
			event = capture(event)
			if event == nil {
				return nil
			}
		}
		if originalCapture != nil {
			return originalCapture(event)
		}
		return event
	})
	return m
}

func (m *CustomModal) GetForm() *tview.Form {
	return m.form
}
