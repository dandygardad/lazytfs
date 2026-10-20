package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// fileNode is one folder or file in the Unstaged/Staged trees.
type fileNode struct {
	name     string
	path     string // relative path, keeps expand state and cursor across re-renders
	absPath  string // local path, used for ignore
	raw      string // raw tf status line, empty for plain folders
	children []*fileNode
}

func (l *Layout) setupFileTrees() {
	l.cwd, _ = os.Getwd()
	l.collapsedUnstaged = make(map[string]bool)
	l.collapsedStaged = make(map[string]bool)

	if data, err := os.ReadFile(ignoreFilePath()); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				l.ignoredPaths = append(l.ignoredPaths, line)
			}
		}
	}

	l.FilesList = tview.NewTreeView()
	l.FilesList.SetTitle(" Unstaged ").SetBorder(true).SetTitleColor(tcell.ColorYellow)

	l.StagedList = tview.NewTreeView()
	l.StagedList.SetTitle(" Staged ").SetBorder(true).SetTitleColor(tcell.ColorGreen)

	for i, tree := range []*tview.TreeView{l.FilesList, l.StagedList} {
		tree := tree
		panelIndex := i + 1
		isStagedTree := tree == l.StagedList
		collapsed := l.collapsedUnstaged
		if isStagedTree {
			collapsed = l.collapsedStaged
		}

		// Root is hidden, its children are the top level
		tree.SetRoot(tview.NewTreeNode("").AddChild(tview.NewTreeNode("Loading...").SetColor(tcell.ColorGray)))
		tree.SetTopLevel(1).SetGraphicsColor(tcell.ColorGray)

		tree.SetFocusFunc(func() {
			l.currentPanel = panelIndex
			l.MainView.SetTitle(" Main View ")
			if isStagedTree {
				l.BottomBar.SetText(" [q] quit | [?] help | [C] checkin | [enter] diff/expand | [space] unstage | [←/→] collapse/expand | [/] search | [1-6] jump | [r] refresh ")
			} else {
				l.BottomBar.SetText(" [q] quit | [?] help | [C] checkin | [enter] diff/expand | [space] stage | [←/→] collapse/expand | [i] ignore | [I] ignored list | [/] search | [1-6] jump | [r] refresh ")
			}
		})

		setExpanded := func(node *tview.TreeNode, fn *fileNode, expanded bool) {
			node.SetExpanded(expanded)
			node.SetText(nodeLabel(fn, expanded))
			if expanded {
				delete(collapsed, fn.path)
			} else {
				collapsed[fn.path] = true
			}
		}

		tree.SetSelectedFunc(func(node *tview.TreeNode) {
			fn, ok := node.GetReference().(*fileNode)
			if !ok {
				return
			}
			if len(fn.children) > 0 {
				setExpanded(node, fn, !node.IsExpanded())
				return
			}
			filename := extractFilePath(fn.raw)
			l.setMainText("Loading diff for " + filename + "...")
			go func() {
				diffOut, err := l.App.tfsClient.GetDiff(filename)
				l.App.tviewApp.QueueUpdateDraw(func() {
					if err != nil {
						l.setMainText("Error getting diff:\n" + err.Error() + "\n\nOutput:\n" + tview.Escape(diffOut))
					} else if rows, added, removed := parseUnifiedDiff(diffOut); len(rows) > 0 {
						l.DiffView.SetDiff(filepath.Base(filename), rows, added, removed)
						l.MainPages.SwitchToPage("diff")
					} else {
						// No hunks to show side by side (e.g. binary file), show tf output as is
						l.setMainText(colorizeDiff(diffOut))
					}
				})
			}()
		})

		tree.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			node := tree.GetCurrentNode()
			var fn *fileNode
			if node != nil {
				fn, _ = node.GetReference().(*fileNode)
			}

			switch event.Key() {
			case tcell.KeyLeft:
				if fn != nil && len(fn.children) > 0 && node.IsExpanded() {
					setExpanded(node, fn, false)
					return nil
				}
				// Jump to parent folder (path[0] is the hidden root)
				if node != nil {
					if path := tree.GetPath(node); len(path) > 2 {
						tree.SetCurrentNode(path[len(path)-2])
					}
				}
				return nil
			case tcell.KeyRight:
				if fn != nil && len(fn.children) > 0 {
					if !node.IsExpanded() {
						setExpanded(node, fn, true)
					} else {
						tree.SetCurrentNode(node.GetChildren()[0])
					}
				}
				return nil
			case tcell.KeyRune:
				switch event.Rune() {
				case ' ':
					if fn == nil {
						return nil
					}
					// Stage/unstage the file, or every file under the folder
					var raws []string
					var collect func(n *fileNode)
					collect = func(n *fileNode) {
						if n.raw != "" {
							raws = append(raws, n.raw)
						}
						for _, c := range n.children {
							collect(c)
						}
					}
					collect(fn)

					l.mu.Lock()
					for _, raw := range raws {
						if isStagedTree {
							delete(l.stagedFiles, raw)
						} else {
							l.stagedFiles[raw] = true
						}
					}
					l.mu.Unlock()
					l.renderFilesPanel()
					return nil
				case '/':
					if isStagedTree {
						l.SearchInput.SetLabel("Search Staged: ")
						l.SearchInput.SetText(l.searchQueryStaged)
					} else {
						l.SearchInput.SetLabel("Search Unstaged: ")
						l.SearchInput.SetText(l.searchQueryUnstaged)
					}
					l.BottomPages.SwitchToPage("search")
					l.App.tviewApp.SetFocus(l.SearchInput)
					return nil
				case 'i':
					if isStagedTree || fn == nil {
						return nil
					}
					l.ignoredPaths = append(l.ignoredPaths, fn.absPath)
					l.renderFilesPanel()
					l.MainView.SetTitle(" Main View ")
					l.setMainText("Hidden from Unstaged:\n" + fn.absPath + "\n\nPress [I] to manage ignored paths.")
					l.saveIgnoredPaths()
					return nil
				}
			}
			return event
		})
	}

	// Ignored paths manager
	l.IgnoreList = tview.NewList().ShowSecondaryText(false)
	l.IgnoreList.SetTitle(" Ignored in Unstaged ").SetBorder(true).SetTitleColor(tcell.ColorGray)

	removeSelected := func() {
		idx := l.IgnoreList.GetCurrentItem()
		if idx < 0 || idx >= len(l.ignoredPaths) {
			return
		}
		l.ignoredPaths = append(l.ignoredPaths[:idx], l.ignoredPaths[idx+1:]...)
		l.saveIgnoredPaths()
		l.refreshIgnoreList()
		l.renderFilesPanel()
	}
	l.IgnoreList.SetSelectedFunc(func(i int, mainText string, secondaryText string, shortcut rune) {
		removeSelected()
	})
	l.IgnoreList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			l.Pages.HidePage("ignore_page")
			l.App.tviewApp.SetFocus(l.panels[l.currentPanel])
			return nil
		}
		if event.Key() == tcell.KeyRune && event.Rune() == 'd' {
			removeSelected()
			return nil
		}
		return event
	})

	inner := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(l.IgnoreList, 0, 1, true).
		AddItem(tview.NewTextView().SetText(" [enter/d] remove | [esc] close "), 1, 0, false)

	l.IgnorePage = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(nil, 0, 1, false).
			AddItem(inner, 80, 1, true).
			AddItem(nil, 0, 1, false), 20, 1, true).
		AddItem(nil, 0, 1, false)
}

func (l *Layout) refreshIgnoreList() {
	l.IgnoreList.Clear()
	for _, p := range l.ignoredPaths {
		l.IgnoreList.AddItem(p, "", 0, nil)
	}
	if len(l.ignoredPaths) == 0 {
		l.IgnoreList.AddItem("(None) - press [i] on an unstaged file/folder to ignore it", "", 0, nil)
	}
}

// ignoreFilePath is shared by all workspaces since it stores absolute local paths.
func ignoreFilePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "lazytfs", "ignore.txt")
}

func (l *Layout) saveIgnoredPaths() {
	path := ignoreFilePath()
	if path == "" {
		l.setMainText("Failed to save ignore list: config dir not found")
		return
	}
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(strings.Join(l.ignoredPaths, "\n")), 0644); err != nil {
		l.setMainText("Failed to save ignore list:\n" + err.Error())
	}
}

func (l *Layout) renderFilesPanel() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.filesData == nil {
		return // still loading
	}

	if l.filesData[0] == "(None)" || l.filesData[0] == "Error:" {
		l.FilesList.SetTitle(" Unstaged ")
		l.StagedList.SetTitle(" Staged ")
		renderTree(l.FilesList, nil, nil, strings.Join(l.filesData, " "))
		renderTree(l.StagedList, nil, nil, "(None)")
		return
	}

	unstagedRoot := &fileNode{}
	stagedRoot := &fileNode{}
	ignoredCount := 0
	for _, raw := range l.filesData {
		if !strings.Contains(raw, `:\`) {
			continue // server folder header like "$/Proj/folder", the tree is built from local paths instead
		}

		absPath := extractFilePath(raw)
		rel := absPath
		base := ""
		if r, err := filepath.Rel(l.cwd, absPath); err == nil && !strings.HasPrefix(r, "..") {
			rel = r
			base = l.cwd
		}

		if l.stagedFiles[raw] {
			if l.searchQueryStaged != "" && !strings.Contains(strings.ToLower(rel), l.searchQueryStaged) {
				continue
			}
			addFileNode(stagedRoot, rel, base, raw)
			continue
		}

		ignored := false
		lowerAbs := strings.ToLower(absPath)
		for _, ig := range l.ignoredPaths {
			lowerIg := strings.ToLower(ig)
			if lowerAbs == lowerIg || strings.HasPrefix(lowerAbs, lowerIg+`\`) {
				ignored = true
				break
			}
		}
		if ignored {
			ignoredCount++
			continue
		}
		if l.searchQueryUnstaged != "" && !strings.Contains(strings.ToLower(rel), l.searchQueryUnstaged) {
			continue
		}
		addFileNode(unstagedRoot, rel, base, raw)
	}

	l.ignoredCount = ignoredCount
	if ignoredCount > 0 {
		l.FilesList.SetTitle(" Unstaged (" + strconv.Itoa(ignoredCount) + " ignored) ")
	} else {
		l.FilesList.SetTitle(" Unstaged ")
	}
	l.StagedList.SetTitle(" Staged ")
	renderTree(l.FilesList, unstagedRoot, l.collapsedUnstaged, "(None)")
	renderTree(l.StagedList, stagedRoot, l.collapsedStaged, "(None)")
}

// addFileNode inserts a file into the tree, creating folder nodes along its path.
// base is the folder rel is relative to, or "" when rel is already absolute.
func addFileNode(root *fileNode, rel, base, raw string) {
	parts := strings.FieldsFunc(rel, func(r rune) bool { return r == '\\' || r == '/' })
	cur := root
	for i, part := range parts {
		var next *fileNode
		for _, c := range cur.children {
			if strings.EqualFold(c.name, part) {
				next = c
				break
			}
		}
		if next == nil {
			path := strings.Join(parts[:i+1], `\`)
			absPath := path
			if base != "" {
				absPath = filepath.Join(base, path)
			}
			next = &fileNode{name: part, path: path, absPath: absPath}
			cur.children = append(cur.children, next)
		}
		cur = next
	}
	cur.raw = raw
}

// renderTree rebuilds a tree view from root and keeps the cursor on the same item,
// or on the next one when it disappeared (e.g. right after staging it).
func renderTree(tree *tview.TreeView, root *fileNode, collapsed map[string]bool, emptyText string) {
	currentPath := ""
	if cur := tree.GetCurrentNode(); cur != nil {
		if fn, ok := cur.GetReference().(*fileNode); ok {
			currentPath = fn.path
		}
	}
	var oldPaths []string
	tree.GetRoot().Walk(func(node, parent *tview.TreeNode) bool {
		if fn, ok := node.GetReference().(*fileNode); ok {
			oldPaths = append(oldPaths, fn.path)
		}
		return node.IsExpanded()
	})

	treeRoot := tview.NewTreeNode("")
	visible := make(map[string]*tview.TreeNode)
	if root == nil || len(root.children) == 0 {
		treeRoot.AddChild(tview.NewTreeNode(emptyText).SetColor(tcell.ColorGray))
	} else {
		addTreeNodes(treeRoot, root, collapsed, visible, true)
	}
	tree.SetRoot(treeRoot)

	var target *tview.TreeNode
	for i, p := range oldPaths {
		if p != currentPath {
			continue
		}
		for _, next := range oldPaths[i:] {
			if target = visible[next]; target != nil {
				break
			}
		}
		for j := i - 1; j >= 0 && target == nil; j-- {
			target = visible[oldPaths[j]]
		}
		break
	}
	if target == nil {
		target = treeRoot.GetChildren()[0]
	}
	tree.SetCurrentNode(target)
}

// addTreeNodes converts n's children into tree nodes. Folders come first, and a folder
// that only contains one folder is merged with it (e.g. "src\app\models").
func addTreeNodes(parent *tview.TreeNode, n *fileNode, collapsed map[string]bool, visible map[string]*tview.TreeNode, isVisible bool) {
	sort.SliceStable(n.children, func(i, j int) bool {
		iDir, jDir := len(n.children[i].children) > 0, len(n.children[j].children) > 0
		if iDir != jDir {
			return iDir
		}
		return strings.ToLower(n.children[i].name) < strings.ToLower(n.children[j].name)
	})

	for _, child := range n.children {
		for child.raw == "" && len(child.children) == 1 && len(child.children[0].children) > 0 {
			only := child.children[0]
			child = &fileNode{name: child.name + `\` + only.name, path: only.path, absPath: only.absPath, raw: only.raw, children: only.children}
		}

		node := tview.NewTreeNode("").SetReference(child)
		if len(child.children) > 0 {
			expanded := !collapsed[child.path]
			node.SetText(nodeLabel(child, expanded)).SetColor(tcell.ColorBlue).SetExpanded(expanded)
			addTreeNodes(node, child, collapsed, visible, isVisible && expanded)
		} else {
			node.SetText(nodeLabel(child, true))
			_, status := parseTfStatusLine(child.raw)
			status = strings.ToLower(status)
			if strings.Contains(status, "add") {
				node.SetColor(tcell.ColorGreen)
			} else if strings.Contains(status, "delete") {
				node.SetColor(tcell.ColorRed)
			} else {
				node.SetColor(tcell.ColorWhite)
			}
		}
		if isVisible {
			visible[child.path] = node
		}
		parent.AddChild(node)
	}
}

// nodeLabel returns "▾ folder" / "▸ folder" for folders and "file.cs (Edit)" for files.
func nodeLabel(fn *fileNode, expanded bool) string {
	label := fn.name
	if fn.raw != "" {
		_, status := parseTfStatusLine(fn.raw)
		if status != "" {
			parts := strings.Fields(strings.ReplaceAll(status, ",", " "))
			for i, p := range parts {
				if p == "add" {
					p = "new"
				}
				parts[i] = strings.ToUpper(p[:1]) + p[1:]
			}
			label += " (" + strings.Join(parts, ", ") + ")"
		}
	}
	if len(fn.children) > 0 {
		if expanded {
			return "▾ " + label
		}
		return "▸ " + label
	}
	return label
}
