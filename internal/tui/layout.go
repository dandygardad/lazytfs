package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Layout struct {
	App            *App
	Pages          *tview.Pages
	HelpModal      *tview.Modal
	DirTree        *tview.TreeView
	TreeModal      *tview.Flex
	GetLatestModal *tview.Modal
	ConflictModal  *tview.Modal
	LoadingModal   *tview.Modal
	ConflictsList  *tview.List
	ConflictPage   *tview.Flex
	Flex           *tview.Flex
	MainView       *tview.TextView
	CommandLog     *tview.TextView
	WorkspaceView  *tview.TextView
	FilesList      *tview.List
	StagedList     *tview.List
	HistoryList    *tview.List
	BottomPages    *tview.Pages
	BottomBar      *tview.TextView
	SearchInput    *tview.InputField

	panels             []tview.Primitive
	currentPanel       int
	searchQueryFiles   string
	searchQueryHistory string
	selectedPath       string
	selectedConflict   string

	// Caches
	filesData     []string
	workspaceData []string
	historyData   []string

	stagedFiles  map[string]bool
	showSplash   bool
	isRefreshing bool

	mu sync.Mutex // Protects caches
}

func NewLayout(app *App) *Layout {
	l := &Layout{
		App:         app,
		stagedFiles: make(map[string]bool),
		showSplash:  true,
	}

	l.WorkspaceView = tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
	l.WorkspaceView.SetTitle(" Status ").SetBorder(true).SetTitleColor(tcell.ColorGreen)
	l.WorkspaceView.SetText(" lazytfs - simple ui for tfs")

	l.FilesList = tview.NewList().ShowSecondaryText(false)
	l.FilesList.SetTitle(" Unstaged ").SetBorder(true).SetTitleColor(tcell.ColorYellow)

	l.StagedList = tview.NewList().ShowSecondaryText(false)
	l.StagedList.SetTitle(" Staged ").SetBorder(true).SetTitleColor(tcell.ColorGreen)

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

	l.CommandLog = tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
	l.CommandLog.SetTitle(" Command Log ").SetBorder(true).SetTitleColor(tcell.ColorGray)

	l.BottomPages = tview.NewPages()
	l.BottomBar = tview.NewTextView().SetDynamicColors(false)
	l.BottomBar.SetText(" [q] quit | [?] help | [g] get latest | [c] conflicts | [enter] select | [1-6] jump | [r] refresh ")

	l.SearchInput = tview.NewInputField().
		SetLabel("Search: ").
		SetFieldWidth(0)

	l.SearchInput.SetChangedFunc(func(text string) {
		if l.currentPanel == 1 || l.currentPanel == 2 {
			l.searchQueryFiles = strings.ToLower(text)
			l.renderFilesPanel()
		}
	})

	l.SearchInput.SetDoneFunc(func(key tcell.Key) {
		if l.currentPanel == 3 {
			l.searchQueryHistory = strings.ToLower(l.SearchInput.GetText())
			l.setLoadingState(l.HistoryList)
			go func() {
				outHistory, errHistory := l.App.tfsClient.GetHistory(l.searchQueryHistory)
				l.mu.Lock()
				l.historyData = parseOutput(outHistory, errHistory)
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
		AddItem(l.MainView, 0, 5, false).
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
[ g ] Get Latest
[ c ] Check Conflicts
[ 1-4 ] Jump to left panels
[ 5 ] Jump to Main View
[ 6 ] Jump to Command Log

Panel Specific:
[ Space ] Stage/Unstage file
[ / ] Search files
[ Enter ] View diff / Select`

	l.HelpModal = tview.NewModal().
		SetText(helpText).
		AddButtons([]string{"Close"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			l.Pages.HidePage("help")
			l.App.tviewApp.SetFocus(l.panels[l.currentPanel])
		})

	l.LoadingModal = tview.NewModal().SetText("Loading...")
	l.LoadingModal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		return nil // Block all input
	})

	l.Pages = tview.NewPages()
	l.Pages.AddPage("main", l.Flex, true, true)

	splashText := "- lazytfs -\n[gray::d]simple ui for tfs[-::-]\n\ndandygarda.com"
	splashView := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText(splashText)

	splashFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(splashView, 4, 1, false).
		AddItem(tview.NewBox(), 0, 1, false)

	l.setupGetLatestUI()
	l.setupConflictsUI()

	l.Pages.AddPage("splash", splashFlex, true, true)
	l.Pages.AddPage("help", l.HelpModal, false, false)
	l.Pages.AddPage("getlatest_tree", l.TreeModal, true, false)
	l.Pages.AddPage("getlatest_confirm", l.GetLatestModal, false, false)
	l.Pages.AddPage("conflict_page", l.ConflictPage, true, false)
	l.Pages.AddPage("conflict_modal", l.ConflictModal, false, false)
	l.Pages.AddPage("loading", l.LoadingModal, false, false)

	l.setupKeybindings()
	return l
}

func (l *Layout) setupGetLatestUI() {
	rootDir := "."
	root := tview.NewTreeNode(rootDir).
		SetColor(tcell.ColorRed)
	l.DirTree = tview.NewTreeView().
		SetRoot(root).
		SetCurrentNode(root)
	l.DirTree.SetBorder(true).SetTitle(" Select Folder to Get Latest ")

	l.DirTree.SetFocusFunc(func() {
		l.BottomBar.SetText(" [esc] cancel | [enter] select | [space] expand/collapse ")
	})

	l.TreeModal = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(nil, 0, 1, false).
			AddItem(l.DirTree, 60, 1, true).
			AddItem(nil, 0, 1, false), 20, 1, true).
		AddItem(nil, 0, 1, false)

	add := func(target *tview.TreeNode, path string) {
		files, err := os.ReadDir(path)
		if err != nil {
			return
		}
		for _, file := range files {
			if file.Name() == ".git" || file.Name() == ".antigravitycli" {
				continue
			}

			color := tcell.ColorWhite
			if file.IsDir() {
				color = tcell.ColorBlue
			}

			node := tview.NewTreeNode(file.Name()).
				SetReference(filepath.Join(path, file.Name())).
				SetSelectable(true).
				SetColor(color)
			target.AddChild(node)
		}
	}
	add(root, rootDir)

	l.DirTree.SetSelectedFunc(func(node *tview.TreeNode) {
		reference := node.GetReference()
		if reference == nil {
			l.selectedPath = rootDir
		} else {
			l.selectedPath = reference.(string)
			if len(node.GetChildren()) == 0 {
				add(node, l.selectedPath)
			} else {
				node.SetExpanded(!node.IsExpanded())
			}
		}
	})

	l.DirTree.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEnter {
			node := l.DirTree.GetCurrentNode()
			ref := node.GetReference()
			if ref == nil {
				l.selectedPath = rootDir
			} else {
				l.selectedPath = ref.(string)
			}
			l.GetLatestModal.SetText("Are you sure to get latest this?\n" + l.selectedPath)
			l.Pages.ShowPage("getlatest_confirm")
			l.App.tviewApp.SetFocus(l.GetLatestModal)
			return nil
		}
		if event.Key() == tcell.KeyEsc {
			l.Pages.HidePage("getlatest_tree")
			l.App.tviewApp.SetFocus(l.panels[l.currentPanel])
			return nil
		}
		return event
	})

	l.GetLatestModal = tview.NewModal().
		AddButtons([]string{"ok", "cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "ok" {
				l.Pages.HidePage("getlatest_confirm")
				l.Pages.HidePage("getlatest_tree")
				l.MainView.SetText("Getting latest for " + l.selectedPath + "...")
				l.App.tviewApp.SetFocus(l.panels[l.currentPanel])

				go func() {
					out, err := l.App.tfsClient.GetLatest(l.selectedPath)
					l.App.tviewApp.QueueUpdateDraw(func() {
						if err != nil && strings.Contains(strings.ToLower(out), "conflict") {
							// Show conflicts page
							l.checkAndShowConflicts(l.selectedPath)
						} else {
							if err != nil {
								l.MainView.SetText("Error getting latest:\n" + err.Error() + "\n" + out)
							} else {
								l.MainView.SetText("Get latest success:\n" + out)
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

func (l *Layout) checkAndShowConflicts(path string) {
	l.MainView.SetText("Checking for conflicts in " + path + "...")
	go func() {
		conflicts, err := l.App.tfsClient.GetConflicts(path)
		l.App.tviewApp.QueueUpdateDraw(func() {
			if err != nil {
				l.MainView.SetText("Error checking conflicts:\n" + err.Error())
				return
			}
			if len(conflicts) == 0 {
				l.MainView.SetText("No conflicts detected in " + path)
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

	l.ConflictModal = tview.NewModal().
		AddButtons([]string{"Take Server", "Keep Mine", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
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
			l.MainView.SetText("Resolving conflict for " + l.selectedConflict + "...")

			go func() {
				out, err := l.App.tfsClient.ResolveConflicts(l.selectedConflict, takeServer)
				l.App.tviewApp.QueueUpdateDraw(func() {
					l.Pages.HidePage("loading")
					
					if err != nil {
						l.MainView.SetText("Error resolving conflict:\n" + err.Error() + "\n" + out)
						l.App.tviewApp.SetFocus(l.ConflictsList)
					} else {
						l.MainView.SetText("Conflict resolved:\n" + out)
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
				l.Pages.ShowPage("getlatest_tree")
				l.App.tviewApp.SetFocus(l.DirTree)
				return nil
			case 'c':
				l.checkAndShowConflicts(".")
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
				l.App.tviewApp.SetFocus(l.MainView)
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
		l.mu.Lock()
		data := strings.Join(l.workspaceData, "\n")
		l.mu.Unlock()
		if data == "" {
			data = "Loading..."
		}
		l.MainView.SetTitle(" Status ")
		l.MainView.SetText("Workspace Information:\n\n" + data)
		l.BottomBar.SetText(" [q] quit | [?] help | [g] get latest | [c] conflicts | [1-6] jump | [r] refresh ")
	})

	bindFilesList := func(list *tview.List) {
		list.SetFocusFunc(func() {
			l.MainView.SetTitle(" Main View ")
			l.BottomBar.SetText(" [q] quit | [?] help | [g] get latest | [c] conflicts | [enter] diff | [space] stage/unstage | [/] search | [1-6] jump | [r] refresh ")
		})
		list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Key() == tcell.KeyRune {
				switch event.Rune() {
				case ' ':
					l.toggleStaged(list)
					return nil
				case '/':
					l.SearchInput.SetLabel("Search Files: ")
					l.SearchInput.SetText(l.searchQueryFiles)
					l.BottomPages.SwitchToPage("search")
					l.App.tviewApp.SetFocus(l.SearchInput)
					return nil
				}
			}
			return event
		})
		list.SetSelectedFunc(func(i int, mainText string, secondaryText string, shortcut rune) {
			if mainText == "(None)" || strings.HasPrefix(mainText, "Error:") || strings.HasPrefix(mainText, "Loading") {
				return
			}
			cleanText := mainText
			var filename string
			idx := strings.Index(cleanText, ":\\")
			if idx > 0 {
				filename = strings.TrimSpace(cleanText[idx-1:])
			} else {
				fields := strings.Fields(cleanText)
				if len(fields) > 0 {
					filename = fields[0]
				}
			}
			if filename != "" {
				l.MainView.SetText("Loading diff for " + filename + "...")
				go func() {
					diffOut, err := l.App.tfsClient.GetDiff(filename)
					l.App.tviewApp.QueueUpdateDraw(func() {
						if err != nil {
							l.MainView.SetText("Error getting diff:\n" + err.Error() + "\n\nOutput:\n" + tview.Escape(diffOut))
						} else {
							l.MainView.SetText(colorizeDiff(diffOut))
						}
					})
				}()
			}
		})
	}

	bindFilesList(l.FilesList)
	bindFilesList(l.StagedList)

	l.HistoryList.SetFocusFunc(func() {
		l.MainView.SetTitle(" Main View ")
		l.BottomBar.SetText(" [q] quit | [?] help | [g] get latest | [c] conflicts | [enter] select | [/] search author | [1-6] jump | [r] refresh ")
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
			}
		}
		return event
	})
	l.HistoryList.SetSelectedFunc(func(i int, mainText string, secondaryText string, shortcut rune) {
		fields := strings.Fields(mainText)
		if len(fields) == 0 {
			return
		}
		changesetID := fields[0]

		l.MainView.SetText("Loading changeset " + changesetID + "...")
		go func() {
			detailOut, detailErr := l.App.tfsClient.GetChangesetDetail(changesetID)

			l.App.tviewApp.QueueUpdateDraw(func() {
				if detailErr != nil {
					l.MainView.SetText("Error getting changeset detail:\n" + detailErr.Error() + "\n\n" + detailOut)
					return
				}

				idx := strings.Index(detailOut, "Items:")
				if idx != -1 {
					detailOut = strings.TrimSpace(detailOut[:idx])
				}

				l.MainView.SetText(tview.Escape(detailOut))
			})

			var collectionURL string
			l.mu.Lock()
			for _, line := range l.workspaceData {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "Collection:") {
					collectionURL = strings.TrimSpace(strings.TrimPrefix(trimmed, "Collection:"))
					break
				}
			}
			l.mu.Unlock()

			if collectionURL != "" {
				url := strings.TrimRight(collectionURL, "/") + "/_versionControl/changeset/" + changesetID
				switch runtime.GOOS {
				case "windows":
					exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
				case "darwin":
					exec.Command("open", url).Start()
				case "linux":
					exec.Command("xdg-open", url).Start()
				}
			}
		}()
	})

	l.MainView.SetFocusFunc(func() {
		l.BottomBar.SetText(" [q] quit | [?] help | [g] get latest | [c] conflicts | [1-6] jump | [r] refresh ")
	})

	l.CommandLog.SetFocusFunc(func() {
		l.BottomBar.SetText(" [q] quit | [?] help | [g] get latest | [c] conflicts | [1-6] jump | [r] refresh ")
	})
}

func (l *Layout) toggleStaged(list *tview.List) {
	l.mu.Lock()
	idx := list.GetCurrentItem()
	if idx >= 0 && idx < list.GetItemCount() {
		mainText, _ := list.GetItemText(idx)
		if mainText != "(None)" && !strings.HasPrefix(mainText, "Error:") {
			l.stagedFiles[mainText] = !l.stagedFiles[mainText]
		}
	}
	l.mu.Unlock()

	idx = list.GetCurrentItem() // save before re-render
	l.renderFilesPanel()
	// Restore index
	if idx >= list.GetItemCount() {
		idx = list.GetItemCount() - 1
	}
	if idx >= 0 {
		list.SetCurrentItem(idx)
	}
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
	l.filesData = nil
	l.workspaceData = nil
	l.historyData = nil
	l.mu.Unlock()

	l.WorkspaceView.SetText(" Loading...")
	l.setLoadingState(l.FilesList)
	l.setLoadingState(l.StagedList)
	l.setLoadingState(l.HistoryList)

	go l.fetchData()
}

func (l *Layout) setLoadingState(list *tview.List) {
	list.Clear()
	list.AddItem("Loading...", "", 0, nil)
}

func (l *Layout) fetchData() {
	// Workspace
	outWorkspace, errWorkspace := l.App.tfsClient.GetWorkspace()
	l.mu.Lock()
	l.workspaceData = parseOutput(outWorkspace, errWorkspace)
	l.mu.Unlock()
	l.App.tviewApp.QueueUpdateDraw(func() {
		l.WorkspaceView.SetText(" lazytfs - simple ui for tfs")

		// If currently focused, trigger focus func to update main view
		if l.App.tviewApp.GetFocus() == l.WorkspaceView {
			data := strings.Join(l.workspaceData, "\n")
			l.MainView.SetTitle(" Status ")
			l.MainView.SetText("Workspace Information:\n\n" + data)
		}
	})

	// Files
	outFiles, errFiles := l.App.tfsClient.GetStatus()
	l.mu.Lock()
	l.filesData = parseOutput(outFiles, errFiles)
	l.mu.Unlock()
	l.App.tviewApp.QueueUpdateDraw(func() {
		l.renderFilesPanel()
	})

	// History
	outHistory, errHistory := l.App.tfsClient.GetHistory(l.searchQueryHistory)
	l.mu.Lock()
	l.historyData = parseOutput(outHistory, errHistory)
	l.mu.Unlock()

	if l.showSplash {
		l.showSplash = false
	}

	l.App.tviewApp.QueueUpdateDraw(func() {
		l.renderHistoryPanel()
		l.Pages.RemovePage("splash")
		l.mu.Lock()
		l.isRefreshing = false
		l.mu.Unlock()
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

		if strings.HasSuffix(trimmed, " item(s)") || trimmed == "No pending changes." || strings.HasPrefix(trimmed, "There are no ") {
			continue
		}

		data = append(data, trimmed)
	}
	if len(data) == 0 {
		return []string{"(None)"}
	}
	return data
}

func (l *Layout) renderFilesPanel() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.filesData == nil {
		return // still loading
	}

	l.FilesList.Clear()
	l.StagedList.Clear()

	for _, item := range l.filesData {
		if item == "(None)" || strings.HasPrefix(item, "Error:") {
			l.FilesList.AddItem(item, "", 0, nil)
			continue
		}

		if l.searchQueryFiles != "" && !strings.Contains(strings.ToLower(item), l.searchQueryFiles) {
			continue
		}

		if l.stagedFiles[item] {
			l.StagedList.AddItem(item, "", 0, nil)
		} else {
			l.FilesList.AddItem(item, "", 0, nil)
		}
	}

	if l.FilesList.GetItemCount() == 0 {
		l.FilesList.AddItem("(None)", "", 0, nil)
	}
	if l.StagedList.GetItemCount() == 0 {
		l.StagedList.AddItem("(None)", "", 0, nil)
	}
}

func (l *Layout) renderHistoryPanel() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.historyData == nil {
		return
	}

	l.HistoryList.Clear()
	for _, item := range l.historyData {
		if item == "(None)" || strings.HasPrefix(item, "Error:") {
			l.HistoryList.AddItem(item, "", 0, nil)
			continue
		}

		l.HistoryList.AddItem(item, "", 0, nil)
	}

	if l.HistoryList.GetItemCount() == 0 {
		l.HistoryList.AddItem("(None)", "", 0, nil)
	}
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
