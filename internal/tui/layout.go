package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dandygardad/lazytfs/internal/tfs"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Layout struct {
	App            *App
	Pages          *tview.Pages
	HelpModal      *CustomModal
	DirTree        *tview.TreeView
	TreeModal      *tview.Flex
	GetLatestModal *CustomModal
	ConflictModal  *CustomModal
	LoadingModal   *CustomModal
	CheckinPage      *tview.Flex
	CheckinInput     *tview.TextArea
	CheckinPreview   *tview.TextView
	CheckinConfirm1  *CustomModal
	CheckinConfirm2  *tview.Flex
	ConflictsList  *tview.List
	ConflictPage   *tview.Flex
	Flex           *tview.Flex
	MainView       *tview.TextView
	DiffView       *DiffView
	MainPages      *tview.Pages // "text" (MainView) or "diff" (DiffView)

	// Changeset page
	ChangesetPage      *tview.Flex
	ChangesetHeader    *tview.TextView
	ChangesetFiles     *tview.List
	ChangesetDiff      *DiffView
	changesetID        int
	changesetItems     []changesetItem
	changesetDiffCache map[int]changesetDiff
	changesetSeq       atomic.Int64 // latest diff request, older results are dropped

	CommandLog     *tview.TextView
	WorkspaceView  *tview.TextView
	FilesList      *tview.TreeView
	StagedList     *tview.TreeView
	IgnoreList     *tview.List
	IgnorePage     *tview.Flex
	HistoryList    *tview.List
	BottomPages    *tview.Pages
	BottomBar      *tview.TextView
	SearchInput    *tview.InputField

	panels             []tview.Primitive
	currentPanel       int
	searchQueryUnstaged string
	searchQueryStaged   string
	searchQueryHistory string
	getLatestChecked   map[string]bool // paths checked in the Get Latest tree
	getLatestPaths     []string        // paths waiting for get latest confirmation
	selectedConflict   string

	// Caches
	filesData     []string
	workspaceData []string
	historyData   []string

	historyHasMore bool // last history page was full, so older changesets may exist

	stagedFiles  map[string]bool
	ignoredCount int // unstaged files hidden by the ignore list
	isRefreshing bool
	stepsDone    [3]bool // workspace, pending changes, history (for loading progress)
	splashView   *tview.TextView

	// File trees
	cwd               string
	ignoredPaths      []string        // local paths hidden from Unstaged
	collapsedUnstaged map[string]bool // collapsed folder paths
	collapsedStaged   map[string]bool

	mu sync.Mutex // Protects caches
}

func NewLayout(app *App) *Layout {
	l := &Layout{
		App:         app,
		stagedFiles: make(map[string]bool),
	}

	l.WorkspaceView = tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
	l.WorkspaceView.SetTitle(" Status ").SetBorder(true).SetTitleColor(tcell.ColorGreen)
	l.WorkspaceView.SetText(" lazytfs - simple ui for tfs")

	l.setupFileTrees()

	l.HistoryList = tview.NewList().ShowSecondaryText(false)
	l.HistoryList.SetTitle(" Changesets ").SetBorder(true).SetTitleColor(tcell.ColorWhite)

	l.panels = []tview.Primitive{
		l.WorkspaceView,
		l.FilesList,
		l.StagedList,
		l.HistoryList,
	}

	l.MainView = tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
	l.MainView.SetTitle(" Main View ").SetBorder(true).SetTitleColor(tcell.ColorWhite)

	l.DiffView = NewDiffView()
	l.MainPages = tview.NewPages().
		AddPage("text", l.MainView, true, true).
		AddPage("diff", l.DiffView, true, false)

	l.CommandLog = tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
	l.CommandLog.SetTitle(" Command Log ").SetBorder(true).SetTitleColor(tcell.ColorGray)

	l.BottomPages = tview.NewPages()
	l.BottomBar = tview.NewTextView().SetDynamicColors(false)
	l.BottomBar.SetText(" [q] quit | [?] help | [g] get latest | [c] conflicts | [C] checkin | [enter] select | [1-6] jump | [r] refresh ")

	l.SearchInput = tview.NewInputField().
		SetLabel("Search: ").
		SetFieldWidth(0)

	l.SearchInput.SetChangedFunc(func(text string) {
		if l.currentPanel == 1 {
			l.searchQueryUnstaged = strings.ToLower(text)
			l.renderFilesPanel()
		} else if l.currentPanel == 2 {
			l.searchQueryStaged = strings.ToLower(text)
			l.renderFilesPanel()
		}
	})

	l.SearchInput.SetDoneFunc(func(key tcell.Key) {
		if l.currentPanel == 3 {
			l.searchQueryHistory = strings.ToLower(l.SearchInput.GetText())
			l.setLoadingState(l.HistoryList)
			go func() {
				outHistory, errHistory := l.App.tfsClient.GetHistory(l.searchQueryHistory, 0)
				l.mu.Lock()
				l.historyData = parseOutput(outHistory, errHistory)
				l.historyHasMore = changesetCount(l.historyData) == tfs.HistoryPageSize
				l.mu.Unlock()
				l.App.tviewApp.QueueUpdateDraw(func() {
					l.renderHistoryPanel()
				})
			}()
		}

		l.BottomPages.SwitchToPage("bar")
		l.App.tviewApp.SetFocus(l.panels[l.currentPanel])
	})

	l.BottomPages.AddPage("bar", l.BottomBar, true, true)
	l.BottomPages.AddPage("search", l.SearchInput, true, false)

	// Layout construction
	leftColumn := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(l.WorkspaceView, 3, 0, true).
		AddItem(l.FilesList, 0, 1, false).
		AddItem(l.StagedList, 0, 1, false).
		AddItem(l.HistoryList, 0, 1, false)

	rightColumn := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(l.MainPages, 0, 5, false).
		AddItem(l.CommandLog, 0, 1, false)

	mainRow := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(leftColumn, 0, 3, true).
		AddItem(rightColumn, 0, 7, false)

	l.Flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(mainRow, 0, 1, true).
		AddItem(l.BottomPages, 1, 0, false)

	l.App.tfsClient.LogFunc = func(msg string) {
		l.App.tviewApp.QueueUpdateDraw(func() {
			text := l.CommandLog.GetText(false)
			if len(text) > 5000 {
				text = text[len(text)-2500:]
			}
			l.CommandLog.SetText(text + "> " + msg + "\n")
			l.CommandLog.ScrollToEnd()
		})
	}

	helpText := `Global Hotkeys:
[ q ] Quit
[ r ] Refresh
[ ? ] Show this help
[ g ] Get Latest (pick files/folders)
[ c ] Check Conflicts
[ C ] Checkin Staged Files
[ 1-4 ] Jump to left panels
[ 5 ] Jump to Main View
[ 6 ] Jump to Command Log

Panel Specific:
[ Space ] Stage/Unstage file or whole folder
[ Left/Right ] Collapse/Expand folder
[ i ] Ignore file/folder in Unstaged
[ I ] Manage ignored paths
[ / ] Search files
[ Enter ] View diff / Toggle folder / Changeset files & diff
[ o ] Open changeset in browser
[ Ctrl+S ] Submit checkin message`

	l.HelpModal = NewCustomModal(helpText, []string{"Close"}, func(buttonIndex int, buttonLabel string) {
		l.Pages.HidePage("help")
		l.App.tviewApp.SetFocus(l.panels[l.currentPanel])
	})

	l.LoadingModal = NewCustomModal("Loading...", nil, nil)
	l.LoadingModal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		return nil // Block all input
	})

	l.Pages = tview.NewPages()
	l.Pages.AddPage("main", l.Flex, true, true)

	splashText := "- lazytfs -\n[gray::d]simple ui for tfs[-::-]\n\ndandygarda.com"
	l.splashView = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText(splashText)

	splashFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(l.splashView, 13, 1, false).
		AddItem(tview.NewBox(), 0, 1, false)

	l.setupGetLatestUI()
	l.setupChangesetUI()
	l.setupConflictsUI()
	l.setupCheckinUI()

	l.Pages.AddPage("splash", splashFlex, true, true)
	l.Pages.AddPage("help", l.HelpModal.Flex, true, false)
	l.Pages.AddPage("getlatest_tree", l.TreeModal, true, false)
	l.Pages.AddPage("getlatest_confirm", l.GetLatestModal.Flex, true, false)
	l.Pages.AddPage("conflict_page", l.ConflictPage, true, false)
	l.Pages.AddPage("conflict_modal", l.ConflictModal.Flex, true, false)
	l.Pages.AddPage("loading", l.LoadingModal.Flex, true, false)
	l.Pages.AddPage("checkin_page", l.CheckinPage, true, false)
	l.Pages.AddPage("checkin_confirm1", l.CheckinConfirm1.Flex, true, false)
	l.Pages.AddPage("checkin_confirm2", l.CheckinConfirm2, true, false)
	l.Pages.AddPage("ignore_page", l.IgnorePage, true, false)
	l.Pages.AddPage("changeset_page", l.ChangesetPage, true, false)

	l.setupKeybindings()
	return l
}

// getLatestItem is the reference of a node in the Get Latest tree.
type getLatestItem struct {
	path   string
	isDir  bool
	loaded bool // children already read from disk
}

func (l *Layout) setupGetLatestUI() {
	l.DirTree = tview.NewTreeView()
	l.DirTree.SetGraphicsColor(tcell.ColorGray)
	l.DirTree.SetBorder(true).SetTitle(" Get Latest - pick files/folders ")

	l.DirTree.SetFocusFunc(func() {
		l.BottomBar.SetText(" [space] check/uncheck | [→/←] expand/collapse | [enter] get latest | [esc] cancel ")
	})

	hint := tview.NewTextView().SetText(" [space] check/uncheck | [→/←] expand/collapse | [enter] get latest checked (or current) | [esc] cancel ")
	inner := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(l.DirTree, 0, 1, true).
		AddItem(hint, 1, 0, false)

	l.TreeModal = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(nil, 0, 1, false).
			AddItem(inner, 100, 1, true).
			AddItem(nil, 0, 1, false), 30, 1, true).
		AddItem(nil, 0, 1, false)

	setExpanded := func(node *tview.TreeNode, item *getLatestItem, expanded bool) {
		if expanded && !item.loaded {
			addTreeChildren(node, item.path)
			item.loaded = true
		}
		node.SetExpanded(expanded)
		node.SetText(getLatestLabel(item, l.getLatestChecked[item.path], expanded))
	}

	// Mouse click / selection toggles folders
	l.DirTree.SetSelectedFunc(func(node *tview.TreeNode) {
		item := node.GetReference().(*getLatestItem)
		if item.isDir {
			setExpanded(node, item, !node.IsExpanded())
		}
	})

	l.DirTree.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		node := l.DirTree.GetCurrentNode()
		if node == nil {
			return event
		}
		item := node.GetReference().(*getLatestItem)

		switch event.Key() {
		case tcell.KeyRight:
			if item.isDir {
				if !node.IsExpanded() || !item.loaded {
					setExpanded(node, item, true)
				} else if len(node.GetChildren()) > 0 {
					l.DirTree.SetCurrentNode(node.GetChildren()[0])
				}
			}
			return nil
		case tcell.KeyLeft:
			if item.isDir && node.IsExpanded() && item.path != "." {
				setExpanded(node, item, false)
				return nil
			}
			if path := l.DirTree.GetPath(node); len(path) > 1 {
				l.DirTree.SetCurrentNode(path[len(path)-2])
			}
			return nil
		case tcell.KeyEnter:
			l.getLatestPaths = nil
			for p := range l.getLatestChecked {
				l.getLatestPaths = append(l.getLatestPaths, p)
			}
			if len(l.getLatestPaths) == 0 {
				l.getLatestPaths = []string{item.path}
			}
			sort.Strings(l.getLatestPaths)

			shown := l.getLatestPaths
			more := ""
			if len(shown) > 6 {
				more = "\n...and " + strconv.Itoa(len(shown)-6) + " more"
				shown = shown[:6]
			}
			l.GetLatestModal.SetText("Get latest for:\n" + strings.Join(shown, "\n") + more)
			l.Pages.ShowPage("getlatest_confirm")
			l.App.tviewApp.SetFocus(l.GetLatestModal)
			return nil
		case tcell.KeyEsc:
			l.Pages.HidePage("getlatest_tree")
			l.App.tviewApp.SetFocus(l.panels[l.currentPanel])
			return nil
		case tcell.KeyRune:
			if event.Rune() == ' ' {
				if l.getLatestChecked[item.path] {
					delete(l.getLatestChecked, item.path)
				} else {
					l.getLatestChecked[item.path] = true
				}
				node.SetText(getLatestLabel(item, l.getLatestChecked[item.path], node.IsExpanded()))
				l.DirTree.SetTitle(" Get Latest - " + strconv.Itoa(len(l.getLatestChecked)) + " checked ")
				return nil
			}
		}
		return event
	})

	l.GetLatestModal = NewCustomModal("", []string{"ok", "cancel"}, func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "ok" {
				paths := l.getLatestPaths
				l.Pages.HidePage("getlatest_confirm")
				l.Pages.HidePage("getlatest_tree")
				l.setMainText("Getting latest for " + strings.Join(paths, ", ") + "...")
				l.App.tviewApp.SetFocus(l.panels[l.currentPanel])

				go func() {
					out, err := l.App.tfsClient.GetLatest(paths)
					l.App.tviewApp.QueueUpdateDraw(func() {
						if err != nil && strings.Contains(strings.ToLower(out), "conflict") {
							// Show conflicts page
							conflictPath := "."
							if len(paths) == 1 {
								conflictPath = paths[0]
							}
							l.checkAndShowConflicts(conflictPath)
						} else {
							if err != nil {
								l.setMainText("Error getting latest:\n" + err.Error() + "\n" + out)
							} else {
								l.setMainText("Get latest success:\n" + out)
								l.RefreshAll()
							}
						}
					})
				}()
			} else {
				l.Pages.HidePage("getlatest_confirm")
				l.App.tviewApp.SetFocus(l.DirTree)
			}
		})
}

// addTreeChildren lists the entries of path (folders first) as collapsed child nodes of target.
func addTreeChildren(target *tview.TreeNode, path string) {
	files, err := os.ReadDir(path)
	if err != nil {
		return
	}
	sort.SliceStable(files, func(i, j int) bool {
		return files[i].IsDir() && !files[j].IsDir()
	})
	for _, file := range files {
		if file.Name() == ".git" || file.Name() == "$tf" {
			continue
		}

		item := &getLatestItem{path: filepath.Join(path, file.Name()), isDir: file.IsDir()}
		color := tcell.ColorWhite
		if item.isDir {
			color = tcell.ColorBlue
		}

		node := tview.NewTreeNode(getLatestLabel(item, false, false)).
			SetReference(item).
			SetColor(color).
			SetExpanded(false)
		target.AddChild(node)
	}
}

// getLatestLabel returns e.g. "[x] ▸ src" for folders and "[ ] app.cs" for files.
func getLatestLabel(item *getLatestItem, checked bool, expanded bool) string {
	label := "[ ] "
	if checked {
		label = "[x] "
	}
	name := filepath.Base(item.path)
	if item.path == "." {
		name = ". (whole folder)"
	}
	if item.isDir {
		if expanded {
			return label + "▾ " + name
		}
		return label + "▸ " + name
	}
	return label + name
}

func (l *Layout) checkAndShowConflicts(path string) {
	l.setMainText("Checking for conflicts in " + path + "...")
	go func() {
		conflicts, err := l.App.tfsClient.GetConflicts(path)
		l.App.tviewApp.QueueUpdateDraw(func() {
			if err != nil {
				l.setMainText("Error checking conflicts:\n" + err.Error())
				return
			}
			if len(conflicts) == 0 {
				l.setMainText("No conflicts detected in " + path)
				return
			}

			l.ConflictsList.Clear()
			for i, conflict := range conflicts {
				path := conflict.Path
				if rel, err := filepath.Rel(".", path); err == nil && !strings.HasPrefix(rel, "..") {
					path = rel
				}
				l.ConflictsList.AddItem(path, conflict.Reason, 0, nil)
				if i < len(conflicts)-1 {
					l.ConflictsList.AddItem(" ", "", 0, nil)
				}
			}
			l.Pages.ShowPage("conflict_page")
			l.App.tviewApp.SetFocus(l.ConflictsList)
		})
	}()
}

func (l *Layout) setupConflictsUI() {
	l.ConflictsList = tview.NewList().ShowSecondaryText(true)
	l.ConflictsList.SetMainTextColor(tcell.ColorYellow)
	l.ConflictsList.SetSecondaryTextColor(tcell.ColorGray)
	l.ConflictsList.SetSelectedBackgroundColor(tcell.ColorDarkRed)
	l.ConflictsList.SetSelectedTextColor(tcell.ColorWhite)
	l.ConflictsList.SetTitle(" Conflicted Files (Enter to resolve) ").SetBorder(true).SetTitleColor(tcell.ColorRed)

	l.ConflictsList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			l.Pages.HidePage("conflict_page")
			l.App.tviewApp.SetFocus(l.panels[l.currentPanel])
			return nil
		}
		return event
	})

	var lastSelected int
	l.ConflictsList.SetChangedFunc(func(i int, mainText string, secondaryText string, shortcut rune) {
		if strings.TrimSpace(mainText) == "" {
			if i > lastSelected && i+1 < l.ConflictsList.GetItemCount() {
				l.ConflictsList.SetCurrentItem(i + 1)
			} else if i < lastSelected && i-1 >= 0 {
				l.ConflictsList.SetCurrentItem(i - 1)
			}
		} else {
			lastSelected = i
		}
	})

	l.ConflictsList.SetSelectedFunc(func(i int, mainText string, secondaryText string, shortcut rune) {
		if strings.TrimSpace(mainText) == "" {
			return
		}
		l.selectedConflict = mainText
		l.ConflictModal.SetText("Resolve conflict for:\n" + l.selectedConflict)
		l.Pages.ShowPage("conflict_modal")
		l.App.tviewApp.SetFocus(l.ConflictModal)
	})

	helpText := tview.NewTextView().SetText(" [esc] back to main view | [enter] resolve file ").SetDynamicColors(false)

	l.ConflictPage = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(l.ConflictsList, 0, 1, true).
		AddItem(helpText, 1, 0, false)

	l.ConflictModal = NewCustomModal("", []string{"Take Server", "Keep Mine", "Cancel"}, func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Cancel" {
				l.Pages.HidePage("conflict_modal")
				l.App.tviewApp.SetFocus(l.ConflictsList)
				return
			}

			takeServer := buttonLabel == "Take Server"
			l.Pages.HidePage("conflict_modal")
			l.LoadingModal.SetText("Resolving conflict for " + l.selectedConflict + "...")
			l.Pages.ShowPage("loading")
			l.App.tviewApp.SetFocus(l.LoadingModal)
			l.setMainText("Resolving conflict for " + l.selectedConflict + "...")

			go func() {
				out, err := l.App.tfsClient.ResolveConflicts(l.selectedConflict, takeServer)
				l.App.tviewApp.QueueUpdateDraw(func() {
					l.Pages.HidePage("loading")
					
					if err != nil {
						l.setMainText("Error resolving conflict:\n" + err.Error() + "\n" + out)
						l.App.tviewApp.SetFocus(l.ConflictsList)
					} else {
						l.setMainText("Conflict resolved:\n" + out)
						// Remove from list
						idx := l.ConflictsList.GetCurrentItem()
						l.ConflictsList.RemoveItem(idx)
						
						// Remove spacer if it exists
						if idx < l.ConflictsList.GetItemCount() {
							mainText, _ := l.ConflictsList.GetItemText(idx)
							if strings.TrimSpace(mainText) == "" {
								l.ConflictsList.RemoveItem(idx)
							}
						} else if idx-1 >= 0 {
							mainText, _ := l.ConflictsList.GetItemText(idx - 1)
							if strings.TrimSpace(mainText) == "" {
								l.ConflictsList.RemoveItem(idx - 1)
							}
						}
						
						if l.ConflictsList.GetItemCount() == 0 {
							l.Pages.HidePage("conflict_page")
							l.App.tviewApp.SetFocus(l.panels[l.currentPanel])
							l.RefreshAll()
						} else {
							l.App.tviewApp.SetFocus(l.ConflictsList)
						}
					}
				})
			}()
		})
}

func (l *Layout) setupKeybindings() {
	// Global Keybindings
	l.Flex.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if l.App.tviewApp.GetFocus() == l.SearchInput {
			return event
		}

		if event.Key() == tcell.KeyRune {
			switch event.Rune() {
			case 'q':
				l.App.tviewApp.Stop()
				return nil
			case '?':
				l.Pages.ShowPage("help")
				l.App.tviewApp.SetFocus(l.HelpModal)
				return nil
			case 'g':
				// Rebuild the tree on every open so new/removed folders show up and checks reset
				l.getLatestChecked = make(map[string]bool)
				rootItem := &getLatestItem{path: ".", isDir: true, loaded: true}
				root := tview.NewTreeNode(getLatestLabel(rootItem, false, true)).
					SetReference(rootItem).
					SetColor(tcell.ColorRed)
				addTreeChildren(root, ".")
				l.DirTree.SetRoot(root).SetCurrentNode(root)
				l.DirTree.SetTitle(" Get Latest - pick files/folders ")
				l.Pages.ShowPage("getlatest_tree")
				l.App.tviewApp.SetFocus(l.DirTree)
				return nil
			case 'c':
				l.checkAndShowConflicts(".")
				return nil
			case 'C':
				l.showCheckinFlow()
				return nil
			case 'I':
				l.refreshIgnoreList()
				l.Pages.ShowPage("ignore_page")
				l.App.tviewApp.SetFocus(l.IgnoreList)
				return nil
			case '1':
				l.setFocus(0)
				return nil
			case '2':
				l.setFocus(1)
				return nil
			case '3':
				l.setFocus(2)
				return nil
			case '4':
				l.setFocus(3)
				return nil
			case '5':
				// Focus whichever is shown: the text view or the side-by-side diff
				_, front := l.MainPages.GetFrontPage()
				l.App.tviewApp.SetFocus(front)
				return nil
			case '6':
				l.App.tviewApp.SetFocus(l.CommandLog)
				return nil
			case 'r':
				l.RefreshAll()
				return nil
			}
		}
		return event
	})

	l.WorkspaceView.SetFocusFunc(func() {
		l.currentPanel = 0
		l.renderStatusView()
		l.BottomBar.SetText(" [q] quit | [?] help | [g] get latest | [c] conflicts | [C] checkin | [o] open collection | [1-6] jump | [r] refresh ")
	})
	l.WorkspaceView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune && event.Rune() == 'o' {
			url := l.collectionURL()
			if url == "" {
				l.setMainText("Collection URL not found in workspace info.")
				return nil
			}
			openBrowser(url)
			return nil
		}
		return event
	})

	l.HistoryList.SetFocusFunc(func() {
		l.currentPanel = 3
		l.MainView.SetTitle(" Main View ")
		l.BottomBar.SetText(" [q] quit | [?] help | [g] get latest | [c] conflicts | [C] checkin | [enter] changeset diff | [o] open in browser | [/] search author | [1-6] jump | [r] refresh ")
	})
	l.HistoryList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune {
			switch event.Rune() {
			case '/':
				l.SearchInput.SetLabel("Search Author: ")
				l.SearchInput.SetText(l.searchQueryHistory)
				l.BottomPages.SwitchToPage("search")
				l.App.tviewApp.SetFocus(l.SearchInput)
				return nil
			case 'o':
				mainText, _ := l.HistoryList.GetItemText(l.HistoryList.GetCurrentItem())
				changesetID := changesetIDFromText(mainText)
				if changesetID == "" {
					return nil
				}

				collectionURL := l.collectionURL()
				if collectionURL == "" {
					l.setMainText("Collection URL not found in workspace info.")
					return nil
				}
				openBrowser(strings.TrimRight(collectionURL, "/") + "/_versionControl/changeset/" + changesetID)
				return nil
			}
		}
		return event
	})
	l.HistoryList.SetSelectedFunc(func(i int, mainText string, secondaryText string, shortcut rune) {
		if mainText == historyLoadMoreText {
			l.loadMoreHistory()
			return
		}
		changesetID := changesetIDFromText(mainText)
		if changesetID == "" {
			return
		}

		l.showChangeset(changesetID)
	})

	l.MainView.SetFocusFunc(func() {
		l.BottomBar.SetText(" [q] quit | [?] help | [g] get latest | [c] conflicts | [C] checkin | [1-6] jump | [r] refresh ")
	})

	l.DiffView.SetFocusFunc(func() {
		l.BottomBar.SetText(" [↑/↓ j/k] scroll | [pgup/pgdn] page | [home/end] top/bottom | [←/→] scroll sideways | [n/p] next/prev change | [1-6] jump ")
	})

	l.CommandLog.SetFocusFunc(func() {
		l.BottomBar.SetText(" [q] quit | [?] help | [g] get latest | [c] conflicts | [C] checkin | [1-6] jump | [r] refresh ")
	})
}

func (l *Layout) setFocus(index int) {
	if index >= 0 && index < len(l.panels) {
		l.currentPanel = index
		l.App.tviewApp.SetFocus(l.panels[l.currentPanel])
	}
}

func (l *Layout) RefreshAll() {
	l.mu.Lock()
	if l.isRefreshing {
		l.mu.Unlock()
		return
	}
	l.isRefreshing = true
	l.stepsDone = [3]bool{}
	l.filesData = nil
	l.workspaceData = nil
	l.historyData = nil
	l.mu.Unlock()

	l.WorkspaceView.SetText(" Loading...")
	// Keep the file trees as they are while loading so the cursor position survives a refresh
	l.FilesList.SetTitle(" Unstaged (loading...) ")
	l.StagedList.SetTitle(" Staged (loading...) ")
	l.setLoadingState(l.HistoryList)

	go l.fetchData(l.searchQueryHistory)
}

// setMainText shows text in the Main View, switching back from the diff view if needed.
func (l *Layout) setMainText(text string) {
	l.MainPages.SwitchToPage("text")
	l.MainView.SetText(text)
}

func (l *Layout) setLoadingState(list *tview.List) {
	list.Clear()
	list.AddItem("Loading...", "", 0, nil)
}

// fetchData runs workfold, status and history in parallel since each tf call is slow.
func (l *Layout) fetchData(historyAuthor string) {
	var wg sync.WaitGroup
	wg.Add(3)
	stepDone := func(i int) {
		l.mu.Lock()
		l.stepsDone[i] = true
		l.mu.Unlock()
		wg.Done()
	}

	// Animate loading progress. tf.exe doesn't report progress, so it's based on
	// how many of the 3 commands are done (splash on first load, Status panel on refresh).
	start := time.Now()
	stop := make(chan struct{})
	go func() {
		spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for frame := 0; ; frame++ {
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
			spin := spinner[frame%len(spinner)]
			elapsed := time.Since(start).Truncate(100 * time.Millisecond).String()
			l.App.tviewApp.QueueUpdateDraw(func() {
				l.mu.Lock()
				defer l.mu.Unlock()
				if !l.isRefreshing {
					return
				}
				done := 0
				steps := ""
				for i, name := range []string{"Workspace info ", "Pending changes", "History        "} {
					if l.stepsDone[i] {
						done++
						steps += "[green]✓[-] " + name + "\n"
					} else {
						steps += "[yellow]" + spin + "[-] " + name + "\n"
					}
				}
				bar := strings.Repeat("█", done*8) + strings.Repeat("░", (3-done)*8)
				l.splashView.SetText("- lazytfs -\n[gray::d]simple ui for tfs[-::-]\n\n" +
					bar + " " + strconv.Itoa(done*100/3) + "%\n\n" +
					steps + "[gray]" + elapsed + "[-]\n\ndandygarda.com")
				l.WorkspaceView.SetText(" " + spin + " Loading " + strconv.Itoa(done) + "/3 (" + elapsed + ")")
			})
		}
	}()

	// Workspace
	go func() {
		defer stepDone(0)
		outWorkspace, errWorkspace := l.App.tfsClient.GetWorkspace()
		if errWorkspace != nil && strings.Contains(strings.ToLower(outWorkspace), "unable to determine") {
			l.App.tviewApp.QueueUpdateDraw(func() {
				modal := NewCustomModal("This project is not registered in TFS project, please configure via Visual studio", []string{"OK"}, func(buttonIndex int, buttonLabel string) {
					l.App.tviewApp.Stop()
				})
				l.Pages.AddPage("not_registered", modal.Flex, true, true)
				l.App.tviewApp.SetFocus(modal)
			})
			return
		}
		l.mu.Lock()
		l.workspaceData = parseOutput(outWorkspace, errWorkspace)
		l.mu.Unlock()
		l.App.tviewApp.QueueUpdateDraw(func() {
			if l.App.tviewApp.GetFocus() == l.WorkspaceView {
				l.renderStatusView()
			}
		})
	}()

	// Files
	go func() {
		defer stepDone(1)
		outFiles, errFiles := l.App.tfsClient.GetStatus()
		l.mu.Lock()
		l.filesData = parseOutput(outFiles, errFiles)
		// Drop staged entries that no longer exist in the current status
		current := make(map[string]bool)
		for _, item := range l.filesData {
			current[item] = true
		}
		for k := range l.stagedFiles {
			if !current[k] {
				delete(l.stagedFiles, k)
			}
		}
		l.mu.Unlock()
		l.App.tviewApp.QueueUpdateDraw(func() {
			l.renderFilesPanel()
		})
	}()

	// History
	go func() {
		defer stepDone(2)
		outHistory, errHistory := l.App.tfsClient.GetHistory(historyAuthor, 0)
		l.mu.Lock()
		l.historyData = parseOutput(outHistory, errHistory)
		l.historyHasMore = changesetCount(l.historyData) == tfs.HistoryPageSize
		l.mu.Unlock()
		l.App.tviewApp.QueueUpdateDraw(func() {
			l.renderHistoryPanel()
		})
	}()

	wg.Wait()
	close(stop)

	l.App.tviewApp.QueueUpdateDraw(func() {
		l.WorkspaceView.SetText(" lazytfs - simple ui for tfs")
		l.Pages.RemovePage("splash")
		l.mu.Lock()
		l.isRefreshing = false
		l.mu.Unlock()
		// Pending changes and history are done now, so refresh the dashboard if it's shown
		if l.App.tviewApp.GetFocus() == l.WorkspaceView {
			l.renderStatusView()
		}
	})
}

func parseOutput(out string, err error) []string {
	if err != nil {
		return []string{"Error:", err.Error()}
	}
	var data []string
	lines := strings.Split(out, "\n")
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}

		isSeparator := true
		for _, c := range trimmed {
			if c != '-' && c != ' ' {
				isSeparator = false
				break
			}
		}
		if isSeparator && len(trimmed) > 3 {
			continue
		}

		if i+1 < len(lines) {
			nextTrimmed := strings.TrimSpace(lines[i+1])
			isNextSeparator := true
			for _, c := range nextTrimmed {
				if c != '-' && c != ' ' {
					isNextSeparator = false
					break
				}
			}
			if isNextSeparator && len(nextTrimmed) > 3 {
				continue
			}
		}

		if strings.HasSuffix(trimmed, " item(s)") || strings.HasSuffix(trimmed, " change(s)") || trimmed == "No pending changes." || strings.HasPrefix(trimmed, "There are no ") {
			continue
		}

		data = append(data, trimmed)
	}
	if len(data) == 0 {
		return []string{"(None)"}
	}
	return data
}

func (l *Layout) renderHistoryPanel() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.historyData == nil {
		return
	}

	l.HistoryList.Clear()
	for _, item := range l.historyData {
		l.HistoryList.AddItem(tview.Escape(item), "", 0, nil)
	}
	if l.historyHasMore {
		l.HistoryList.AddItem(historyLoadMoreText, "", 0, nil)
	}

	if l.HistoryList.GetItemCount() == 0 {
		l.HistoryList.AddItem("(None)", "", 0, nil)
	}
}

const historyLoadMoreText = "[yellow]▼ Load more changesets...[-]"

// loadMoreHistory appends the next page of changesets older than the last one shown.
func (l *Layout) loadMoreHistory() {
	l.mu.Lock()
	lastID := 0
	for _, line := range l.historyData {
		if id := changesetIDFromText(line); id != "" {
			lastID, _ = strconv.Atoi(id)
		}
	}
	l.mu.Unlock()
	if lastID <= 1 {
		return
	}

	idx := l.HistoryList.GetCurrentItem()
	l.HistoryList.SetItemText(idx, "[gray]Loading more...[-]", "")
	author := l.searchQueryHistory
	go func() {
		out, err := l.App.tfsClient.GetHistory(author, lastID)
		more := parseOutput(out, err)
		l.mu.Lock()
		count := changesetCount(more)
		if err != nil || count > 0 {
			l.historyData = append(l.historyData, more...)
		}
		l.historyHasMore = count == tfs.HistoryPageSize
		l.mu.Unlock()
		l.App.tviewApp.QueueUpdateDraw(func() {
			l.renderHistoryPanel()
			// The first new changeset takes the place of "Load more"
			l.HistoryList.SetCurrentItem(idx)
		})
	}()
}

// changesetCount counts the changeset rows in parsed tf history output.
func changesetCount(lines []string) int {
	count := 0
	for _, line := range lines {
		if changesetIDFromText(line) != "" {
			count++
		}
	}
	return count
}

func colorizeDiff(diff string) string {
	lines := strings.Split(tview.Escape(diff), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			lines[i] = "[green]" + line + "[-]"
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			lines[i] = "[red]" + line + "[-]"
		} else if strings.HasPrefix(line, "@@") {
			lines[i] = "[cyan]" + line + "[-]"
		}
	}
	return strings.Join(lines, "\n")
}

func (l *Layout) setupCheckinUI() {
	// 1. Input Modal
	l.CheckinInput = tview.NewTextArea().
		SetLabel("Checkin Message: ").
		SetPlaceholder("Enter = new line")

	l.CheckinPreview = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetWordWrap(true)
	l.CheckinPreview.SetTitle(" Files to Checkin ").
		SetBorder(true)

	innerModal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(l.CheckinPreview, 0, 1, false).
		AddItem(tview.NewBox(), 1, 0, false). // Spacer
		AddItem(l.CheckinInput, 5, 0, true).
		AddItem(tview.NewTextView().SetText(" [ctrl+s] submit | [esc] cancel "), 1, 0, false)
	innerModal.SetBorder(true).SetTitle(" Checkin ")

	l.CheckinPage = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(nil, 0, 1, false).
			AddItem(innerModal, 80, 1, true).
			AddItem(nil, 0, 1, false), 24, 1, true).
		AddItem(nil, 0, 1, false)

	// Enter inserts a new line (default TextArea behavior), Ctrl+S submits
	l.CheckinInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlS {
			if strings.TrimSpace(l.CheckinInput.GetText()) == "" {
				return nil
			}
			l.Pages.ShowPage("checkin_confirm1")
			l.App.tviewApp.SetFocus(l.CheckinConfirm1)
			return nil
		} else if event.Key() == tcell.KeyEsc {
			l.Pages.HidePage("checkin_page")
			l.App.tviewApp.SetFocus(l.panels[l.currentPanel])
			return nil
		}
		return event
	})

	// 2. Confirm 1
	l.CheckinConfirm1 = NewCustomModal("Are you sure to checkin this data?", []string{"Yes", "No"}, func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Yes" {
				l.Pages.HidePage("checkin_confirm1")
				l.Pages.ShowPage("checkin_confirm2")
				l.App.tviewApp.SetFocus(l.CheckinConfirm2)
			} else {
				l.Pages.HidePage("checkin_confirm1")
				l.App.tviewApp.SetFocus(l.CheckinInput)
			}
		})

	// 3. Confirm 2 with custom colored buttons
	form := tview.NewForm().
		AddButton("Yes", func() {
			l.Pages.HidePage("checkin_confirm2")
			l.Pages.HidePage("checkin_page")
			
			var filesToCkin []string
			l.mu.Lock()
			for f, staged := range l.stagedFiles {
				if staged {
					if strings.HasSuffix(strings.TrimSpace(f), ":") {
						continue
					}
					extractedPath := extractFilePath(f)
					if extractedPath != "" {
						filesToCkin = append(filesToCkin, extractedPath)
					}
				}
			}
			l.mu.Unlock()

			msg := l.CheckinInput.GetText()

			// Without file args, tf checkin would check in ALL pending changes
			if len(filesToCkin) == 0 {
				l.MainView.SetTitle(" Checkin Error ")
				l.setMainText("No valid file paths found in staged files. Checkin cancelled.")
				l.App.tviewApp.SetFocus(l.panels[l.currentPanel])
				return
			}

			l.LoadingModal.SetText("Checking in files...")
			l.Pages.ShowPage("loading")
			l.App.tviewApp.SetFocus(l.LoadingModal)

			go func() {
				out, err := l.App.tfsClient.Checkin(filesToCkin, msg)
				l.App.tviewApp.QueueUpdateDraw(func() {
					l.Pages.HidePage("loading")
					l.CheckinInput.SetText("", false) // Clear input after checkin attempt

					if err != nil {
						l.MainView.SetTitle(" Checkin Error ")
						l.setMainText(err.Error() + "\n" + out)
					} else {
						l.MainView.SetTitle(" Checkin Success ")
						l.setMainText(out)
						
						// Clear staged files on success
						l.mu.Lock()
						for k := range l.stagedFiles {
							delete(l.stagedFiles, k)
						}
						l.mu.Unlock()
						l.RefreshAll()
					}
					l.App.tviewApp.SetFocus(l.panels[l.currentPanel])
				})
			}()
		}).
		AddButton("No", func() {
			l.Pages.HidePage("checkin_confirm2")
			l.App.tviewApp.SetFocus(l.CheckinInput)
		})
	
	form.SetButtonsAlign(tview.AlignCenter)

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

	// Color the buttons
	buttonYes := form.GetButton(0)
	buttonYes.SetBackgroundColor(tcell.ColorGreen)
	buttonYes.SetLabelColor(tcell.ColorWhite)
	
	buttonNo := form.GetButton(1)
	buttonNo.SetBackgroundColor(tcell.ColorRed)
	buttonNo.SetLabelColor(tcell.ColorWhite)

	textView := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText("\nAre you really sure to checkin this data?\n")

	innerFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(textView, 4, 1, false).
		AddItem(form, 3, 1, true)
	innerFlex.SetBorder(true).SetTitle(" Final Confirmation ")

	l.CheckinConfirm2 = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(nil, 0, 1, false).
			AddItem(innerFlex, 50, 1, true).
			AddItem(nil, 0, 1, false), 9, 1, true).
		AddItem(nil, 0, 1, false)

	// Ensure escape goes back, and allow left/right navigation
	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			l.Pages.HidePage("checkin_confirm2")
			l.App.tviewApp.SetFocus(l.CheckinInput)
			return nil
		} else if event.Key() == tcell.KeyLeft {
			return tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone)
		} else if event.Key() == tcell.KeyRight {
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
		l.MainView.SetTitle(" Error ")
		l.setMainText("No files staged for checkin.")
		return
	}

	l.CheckinPreview.SetText(strings.Join(staged, "\n"))
	l.Pages.ShowPage("checkin_page")
	l.App.tviewApp.SetFocus(l.CheckinInput)
}

// changesetIDFromText returns the changeset ID from a history row, or "" for
// non-changeset rows like "(None)", "Loading...", or error lines.
func changesetIDFromText(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	if _, err := strconv.Atoi(fields[0]); err != nil {
		return ""
	}
	return fields[0]
}

func extractFilePath(cleanText string) string {
	idx := strings.Index(cleanText, ":\\")
	if idx > 0 {
		return strings.TrimSpace(cleanText[idx-1:])
	}
	fields := strings.Fields(cleanText)
	if len(fields) > 0 {
		return fields[0]
	}
	return cleanText
}

func parseTfStatusLine(raw string) (string, string) {
	idx := strings.Index(raw, ":\\")
	if idx > 0 {
		prefix := strings.TrimSpace(raw[:idx-1])
		fields := strings.Fields(prefix)
		if len(fields) > 0 {
			statusIdx := len(fields) - 1
			for i := len(fields) - 1; i >= 0; i-- {
				word := strings.ToLower(strings.Trim(fields[i], ","))
				isStatus := false
				for _, s := range []string{"edit", "add", "delete", "rename", "undelete", "branch", "merge", "lock"} {
					if word == s {
						isStatus = true
						break
					}
				}
				if isStatus {
					statusIdx = i
				} else {
					break
				}
			}
			
			status := strings.Join(fields[statusIdx:], " ")
			filename := strings.Join(fields[:statusIdx], " ")
			
			return filename, status
		}
	}
	return raw, ""
}
