package tui

import (
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// diffRow is one line of the side-by-side diff.
type diffRow struct {
	kind    byte // 'h' header, 'f' file labels, '@' hunk header, 'c' code
	oldNo   int  // line number on the left side, 0 = no line
	newNo   int  // line number on the right side, 0 = no line
	oldText string
	newText string
	removed bool // left side is a removed line
	added   bool // right side is an added line
}

var (
	diffRemovedStyle = tcell.StyleDefault.Background(tcell.NewRGBColor(75, 25, 25)).Foreground(tcell.ColorWhite)
	diffAddedStyle   = tcell.StyleDefault.Background(tcell.NewRGBColor(25, 65, 25)).Foreground(tcell.ColorWhite)
	diffEmptyStyle   = tcell.StyleDefault.Background(tcell.NewRGBColor(35, 35, 35))
	diffLabelStyle   = tcell.StyleDefault.Foreground(tcell.ColorWhite).Bold(true)
)

// DiffView shows a unified diff as two columns: old (left) and new (right).
type DiffView struct {
	*tview.Box
	rows    []diffRow
	offset  int // first visible row
	hOffset int // horizontal scroll in characters
	maxNo   int // biggest line number, for the line number column width

	// IgnoreSpace hides changes that are only whitespace, like Visual Studio's
	// "Ignore white space". Toggled with "w" and shown in the title.
	IgnoreSpace bool
}

func NewDiffView() *DiffView {
	d := &DiffView{Box: tview.NewBox(), IgnoreSpace: true}
	d.SetBorder(true).SetTitleColor(tcell.ColorWhite)
	return d
}

func (d *DiffView) SetDiff(filename string, rows []diffRow, added, removed int) {
	d.rows = rows
	d.offset = 0
	d.hOffset = 0
	d.maxNo = 0
	for _, r := range rows {
		d.maxNo = max(d.maxNo, r.oldNo, r.newNo)
	}
	if filename == "" {
		d.SetTitle(" Diff ")
		return
	}
	mode := "  · whitespace shown "
	if d.IgnoreSpace {
		mode = "  · whitespace ignored "
	}
	d.SetTitle(" Diff: " + filename + "  +" + strconv.Itoa(added) + " -" + strconv.Itoa(removed) + mode)
}

// CurrentLine returns the local (right side) line number at the top of the view,
// so an editor can open where the user is looking. 0 when there is none.
func (d *DiffView) CurrentLine() int {
	for i := d.offset; i < len(d.rows); i++ {
		if d.rows[i].newNo > 0 {
			return d.rows[i].newNo
		}
	}
	return 0
}

// parseUnifiedDiff turns `tf diff /format:Unified` output into side-by-side rows.
// Removed and added lines next to each other are paired on the same row.
// Returns no rows when the output has no hunks (e.g. binary files).
func parseUnifiedDiff(diff string) (rows []diffRow, added, removed int) {
	type numbered struct {
		no   int
		text string
	}
	var dels, adds []numbered
	flush := func() {
		for i := 0; i < len(dels) || i < len(adds); i++ {
			row := diffRow{kind: 'c'}
			if i < len(dels) {
				row.oldNo, row.oldText, row.removed = dels[i].no, dels[i].text, true
			}
			if i < len(adds) {
				row.newNo, row.newText, row.added = adds[i].no, adds[i].text, true
			}
			rows = append(rows, row)
		}
		dels, adds = nil, nil
	}
	// parseRange reads "-10,7" or "+10" into start line and line count
	parseRange := func(s string) (int, int) {
		start, count, found := strings.Cut(s[1:], ",")
		n, _ := strconv.Atoi(start)
		c := 1
		if found {
			c, _ = strconv.Atoi(count)
		}
		return n, c
	}

	oldNo, newNo, oldLeft, newLeft := 0, 0, 0, 0
	pendingOld := ""
	hasHunk := false
	for _, line := range strings.Split(strings.ReplaceAll(diff, "\r\n", "\n"), "\n") {
		line = strings.ReplaceAll(line, "\t", "    ")

		// Inside a hunk, the remaining line counts decide what a line is,
		// so a removed "-- comment" is not mistaken for a "--- file" header.
		if oldLeft > 0 || newLeft > 0 {
			switch {
			case strings.HasPrefix(line, "-"):
				dels = append(dels, numbered{oldNo, line[1:]})
				oldNo++
				oldLeft--
				removed++
				continue
			case strings.HasPrefix(line, "+"):
				adds = append(adds, numbered{newNo, line[1:]})
				newNo++
				newLeft--
				added++
				continue
			case strings.HasPrefix(line, " ") || line == "":
				flush()
				text := strings.TrimPrefix(line, " ")
				rows = append(rows, diffRow{kind: 'c', oldNo: oldNo, newNo: newNo, oldText: text, newText: text})
				oldNo++
				newNo++
				oldLeft--
				newLeft--
				continue
			case strings.HasPrefix(line, `\`):
				continue // "\ No newline at end of file"
			}
		}

		flush()
		switch {
		case strings.HasPrefix(line, "@@"):
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			oldNo, oldLeft = parseRange(fields[1])
			newNo, newLeft = parseRange(fields[2])
			rows = append(rows, diffRow{kind: '@', oldText: line})
			hasHunk = true
		case strings.HasPrefix(line, "--- "):
			pendingOld = line[4:]
		case strings.HasPrefix(line, "+++ "):
			rows = append(rows, diffRow{kind: 'f', oldText: pendingOld, newText: line[4:]})
		case strings.TrimSpace(line) == "" || strings.Trim(line, "=") == "" || strings.HasPrefix(line, `\`):
			// skip blank lines, "=====" separators and "\ No newline at end of file"
		default:
			rows = append(rows, diffRow{kind: 'h', oldText: line})
		}
	}
	flush()

	if !hasHunk {
		return nil, 0, 0
	}
	return rows, added, removed
}

func (d *DiffView) Draw(screen tcell.Screen) {
	d.Box.DrawForSubclass(screen, d)
	x, y, width, height := d.GetInnerRect()
	if width <= 2 || height <= 0 {
		return
	}
	if d.offset > len(d.rows)-height {
		d.offset = len(d.rows) - height
	}
	if d.offset < 0 {
		d.offset = 0
	}

	numW := len(strconv.Itoa(d.maxNo))
	half := (width - 1) / 2
	rightX := x + half + 1
	rightW := width - half - 1
	sepStyle := tcell.StyleDefault.Foreground(tcell.ColorGray)

	for i := 0; i < height && d.offset+i < len(d.rows); i++ {
		row := d.rows[d.offset+i]
		ry := y + i
		switch row.kind {
		case 'h':
			printCells(screen, x, ry, width, row.oldText, tcell.StyleDefault.Foreground(tcell.ColorYellow))
		case '@':
			printCells(screen, x, ry, width, row.oldText, tcell.StyleDefault.Foreground(tcell.ColorDarkCyan))
		case 'f':
			printCells(screen, x, ry, half, row.oldText, diffLabelStyle)
			screen.SetContent(x+half, ry, '│', nil, sepStyle)
			printCells(screen, rightX, ry, rightW, row.newText, diffLabelStyle)
		default:
			leftStyle, rightStyle := tcell.StyleDefault, tcell.StyleDefault
			if row.removed {
				leftStyle = diffRemovedStyle
			} else if row.oldNo == 0 {
				leftStyle = diffEmptyStyle
			}
			if row.added {
				rightStyle = diffAddedStyle
			} else if row.newNo == 0 {
				rightStyle = diffEmptyStyle
			}
			d.drawSide(screen, x, ry, half, numW, row.oldNo, row.oldText, leftStyle)
			screen.SetContent(x+half, ry, '│', nil, sepStyle)
			d.drawSide(screen, rightX, ry, rightW, numW, row.newNo, row.newText, rightStyle)
		}
	}
}

// drawSide draws one half of a code row: background, line number, then the code
// shifted by the horizontal scroll.
func (d *DiffView) drawSide(screen tcell.Screen, x, y, width, numW, no int, text string, style tcell.Style) {
	for i := 0; i < width; i++ {
		screen.SetContent(x+i, y, ' ', nil, style)
	}
	if no == 0 {
		return
	}
	numStr := strconv.Itoa(no)
	printCells(screen, x+numW-len(numStr), y, len(numStr), numStr, style.Foreground(tcell.ColorGray))

	runes := []rune(text)
	if d.hOffset < len(runes) {
		printCells(screen, x+numW+1, y, width-numW-1, string(runes[d.hOffset:]), style)
	}
}

// printCells draws text starting at x, cut at maxWidth screen columns.
func printCells(screen tcell.Screen, x, y, maxWidth int, text string, style tcell.Style) {
	w := 0
	for _, r := range text {
		rw := runewidth.RuneWidth(r)
		if rw == 0 {
			continue
		}
		if w+rw > maxWidth {
			return
		}
		screen.SetContent(x+w, y, r, nil, style)
		w += rw
	}
}

func (d *DiffView) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return d.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		_, _, _, height := d.GetInnerRect()
		switch event.Key() {
		case tcell.KeyUp:
			d.offset--
		case tcell.KeyDown:
			d.offset++
		case tcell.KeyPgUp, tcell.KeyCtrlB:
			d.offset -= height
		case tcell.KeyPgDn, tcell.KeyCtrlF:
			d.offset += height
		case tcell.KeyHome:
			d.offset = 0
		case tcell.KeyEnd:
			d.offset = len(d.rows)
		case tcell.KeyLeft:
			d.hOffset = max(d.hOffset-8, 0)
		case tcell.KeyRight:
			d.hOffset += 8
		case tcell.KeyRune:
			switch event.Rune() {
			case 'k':
				d.offset--
			case 'j':
				d.offset++
			case 'n':
				// Jump to the next block of changes
				i := d.offset + 1
				for i < len(d.rows) && !(d.rows[i].kind == 'c' && (d.rows[i].removed || d.rows[i].added) && !(d.rows[i-1].removed || d.rows[i-1].added)) {
					i++
				}
				if i < len(d.rows) {
					d.offset = i
				}
			case 'p':
				// Jump to the previous block of changes
				i := d.offset - 1
				for i > 0 && !(d.rows[i].kind == 'c' && (d.rows[i].removed || d.rows[i].added) && !(d.rows[i-1].removed || d.rows[i-1].added)) {
					i--
				}
				if i >= 0 {
					d.offset = i
				}
			}
		}
	})
}

func (d *DiffView) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return d.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		if !d.InRect(event.Position()) {
			return false, nil
		}
		switch action {
		case tview.MouseLeftClick:
			setFocus(d)
		case tview.MouseScrollUp:
			d.offset -= 3
		case tview.MouseScrollDown:
			d.offset += 3
		default:
			return false, nil
		}
		return true, nil
	})
}
