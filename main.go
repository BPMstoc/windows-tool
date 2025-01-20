package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const passwordFilePath = "passwords.json" // File path for storing passwords
const encryptionKey = "a16byteslongkey!"  // Must be 16, 24, or 32 bytes long

// ---------------------------------------------------------------------
//  1) Windows Directories
// ---------------------------------------------------------------------

var userProfile = os.Getenv("USERPROFILE")

func init() {
	if userProfile == "" {
		userProfile = `C:\Users\Default`
	}
}

var (
	RECOMM_DIR_DOWNLOADS = filepath.Join(userProfile, "Downloads")
	RECOMM_DIR_TEMP      = `C:\Windows\Temp`
	RECOMM_DIR_PROGRAMS  = `C:\Program Files`
	RECOMM_DIR_APPDATA   = filepath.Join(userProfile, "AppData", "Local")
)

// ---------------------------------------------------------------------
//  2) Custom Dark Theme
// ---------------------------------------------------------------------

type MyDarkTheme struct{}

func (m *MyDarkTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.RGBA{R: 30, G: 30, B: 30, A: 255}
	case theme.ColorNameButton:
		return color.RGBA{R: 45, G: 45, B: 45, A: 255}
	case theme.ColorNameDisabledButton:
		return color.RGBA{R: 70, G: 70, B: 70, A: 255}
	case theme.ColorNameDisabled:
		return color.RGBA{R: 160, G: 160, B: 160, A: 255}
	case theme.ColorNameForeground:
		return color.RGBA{R: 220, G: 220, B: 220, A: 255}
	case theme.ColorNameHover:
		return color.RGBA{R: 50, G: 50, B: 50, A: 255}
	case theme.ColorNameFocus:
		return color.RGBA{R: 255, G: 165, B: 0, A: 180}
	case theme.ColorNameScrollBar:
		return color.RGBA{R: 60, G: 60, B: 60, A: 255}
	}
	return theme.DefaultTheme().Color(name, variant)
}
func (m *MyDarkTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}
func (m *MyDarkTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}
func (m *MyDarkTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 8
	case theme.SizeNameScrollBar:
		return 10
	}
	return theme.DefaultTheme().Size(name)
}

// ---------------------------------------------------------------------
//  3) Data Structures
// ---------------------------------------------------------------------

type DeletionRecord struct {
	Timestamp string
	FilePath  string
	Method    string // e.g. "Duplicate Finder" or "Space Cleaner"
}

type FileItem struct {
	filePath string
	check    *widget.Check
}

type LargeFileItem struct {
	filePath string
	size     int64
	check    *widget.Check
}

type FileScanner struct {
	// Deletion History
	deletionRecords []DeletionRecord

	// Duplicate Finder
	allDuplicates    map[string][]string
	allFileItems     []*FileItem
	lastSelectedSort string

	mainWindow fyne.Window

	duplicateFinderRoot fyne.CanvasObject
	spaceCleanerRoot    fyne.CanvasObject
	historyRoot         fyne.CanvasObject

	split   *container.Split
	leftNav fyne.CanvasObject
	rightUI fyne.CanvasObject

	duplicateListVBox *fyne.Container
	// Pagination for duplicates
	dfPageSize    int
	dfCurrentPage int
	dfTotalPages  int
	dfPageLabel   *widget.Label
	dfPrevBtn     *widget.Button
	dfNextBtn     *widget.Button

	largeFileItems []*LargeFileItem
	// Pagination for space cleaner
	scPageSize    int
	scCurrentPage int
	scTotalPages  int
	scPageLabel   *widget.Label
	scPrevBtn     *widget.Button
	scNextBtn     *widget.Button

	passwordManagerRoot fyne.CanvasObject
}

// ---------------------------------------------------------------------
//  4) main()
// ---------------------------------------------------------------------

func main() {
	a := app.New()
	a.Settings().SetTheme(&MyDarkTheme{})

	w := a.NewWindow("Windows Optimization Tool")
	w.Resize(fyne.NewSize(1200, 700))

	scanner := &FileScanner{
		deletionRecords:  []DeletionRecord{},
		allDuplicates:    map[string][]string{},
		allFileItems:     []*FileItem{},
		largeFileItems:   []*LargeFileItem{},
		lastSelectedSort: "Path",
		mainWindow:       w,
		// 20 per page for both
		dfPageSize: 20,
		scPageSize: 20,
	}

	// Initialize left menu and individual tabs
	scanner.leftNav = scanner.makeLeftMenu()
	scanner.duplicateFinderRoot = scanner.setupDuplicateFinderUI()
	scanner.spaceCleanerRoot = scanner.setupSpaceCleanerUI()
	scanner.historyRoot = scanner.setupHistoryUI()
	scanner.passwordManagerRoot = scanner.setupPasswordManagerUI() // Initialize the Password Manager

	// Set the default right UI
	scanner.rightUI = scanner.duplicateFinderRoot

	// Configure the split layout
	scanner.split = container.NewHSplit(scanner.leftNav, scanner.rightUI)
	scanner.split.Offset = 0.2

	// Set the content and start the app
	w.SetContent(scanner.split)
	w.ShowAndRun()
}

// ---------------------------------------------------------------------
//  5) Left Menu
// ---------------------------------------------------------------------

func (s *FileScanner) makeLeftMenu() fyne.CanvasObject {

	dupBtn := widget.NewButton("Duplicate Finder", func() {
		s.switchRightContent(s.duplicateFinderRoot)
	})
	spaceCleanerBtn := widget.NewButton("Space Cleaner", func() {
		s.switchRightContent(s.spaceCleanerRoot)
	})
	historyBtn := widget.NewButton("Deletion History", func() {
		s.switchRightContent(s.historyRoot)
	})
	passwordManagerBtn := widget.NewButton("Password Manager", func() {
		s.switchRightContent(s.passwordManagerRoot)
	})

	return container.NewVBox(
		dupBtn,
		spaceCleanerBtn,
		historyBtn,
		passwordManagerBtn, // Add the Password Manager button here
		layout.NewSpacer(),
	)
}

func (s *FileScanner) switchRightContent(content fyne.CanvasObject) {
	if s.split == nil {
		return
	}
	s.split.Trailing = content
	s.split.Refresh()
}

// ---------------------------------------------------------------------
//  6) Duplicate Finder UI
// ---------------------------------------------------------------------

func (s *FileScanner) setupDuplicateFinderUI() fyne.CanvasObject {
	// We create multi-line entries, then wrap them in container.NewGridWrap to force bigger size
	dirEntry := widget.NewMultiLineEntry()
	dirEntry.SetPlaceHolder("Enter directory path...")
	dirEntry.Wrapping = fyne.TextWrapWord
	dirWrap := container.NewGridWrap(fyne.NewSize(300, 60), dirEntry)

	filterEntry := widget.NewMultiLineEntry()
	filterEntry.SetPlaceHolder("e.g. .txt,.csv")
	filterEntry.Wrapping = fyne.TextWrapWord
	filterWrap := container.NewGridWrap(fyne.NewSize(200, 60), filterEntry)

	filterLabel := widget.NewLabel("Filter by extension")

	selectDirBtn := widget.NewButton("Select Directory", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			fullURI := uri.String()
			if strings.HasPrefix(fullURI, "file://") {
				fullURI = strings.TrimPrefix(fullURI, "file://")
			}
			dirEntry.SetText(fullURI)
		}, s.mainWindow)
	})

	topBar := container.NewHBox(
		container.NewVBox(
			widget.NewForm(&widget.FormItem{
				Text:   "Directory:",
				Widget: dirWrap,
			}),
		),
		selectDirBtn,
		container.NewVBox(filterLabel, filterWrap),
	)

	s.duplicateListVBox = container.NewVBox()
	scroll := container.NewScroll(s.duplicateListVBox)
	scroll.SetMinSize(fyne.NewSize(0, 380))

	findDuplicatesBtn := widget.NewButton("Find Duplicates", func() {
		s.duplicateListVBox.Objects = nil
		s.allFileItems = nil
		s.allDuplicates = map[string][]string{}
		s.dfCurrentPage = 0

		dirPath := strings.TrimSpace(dirEntry.Text)
		if dirPath == "" {
			dialog.ShowInformation("Error", "Please enter or select a directory.", s.mainWindow)
			return
		}
		s.showScanningDuplicates(dirPath, filterEntry.Text)
	})

	deleteSelectedBtn := widget.NewButton("Delete Selected", func() {
		toDelete := s.getCheckedFiles()
		if len(toDelete) == 0 {
			dialog.ShowInformation("No Files Selected", "Please select at least one file.", s.mainWindow)
			return
		}
		dialog.ShowConfirm("Confirm Deletion", fmt.Sprintf("Delete %d file(s)?", len(toDelete)), func(c bool) {
			if !c {
				return
			}
			var errs []string
			var deletedCount int
			for _, fp := range toDelete {
				err := os.Remove(fp)
				if err != nil {
					errs = append(errs, fmt.Sprintf("Failed to delete %s: %v", fp, err))
				} else {
					deletedCount++
					s.addDeletionRecord(fp, "Duplicate Finder")
				}
			}
			if len(errs) > 0 {
				dialog.ShowError(fmt.Errorf(strings.Join(errs, "\n")), s.mainWindow)
			}
			dialog.ShowInformation("Deletion Complete", fmt.Sprintf("Deleted %d file(s).", deletedCount), s.mainWindow)
			s.refreshDeletionTable()
			s.refreshDuplicates()
		}, s.mainWindow)
	})

	renameBtn := widget.NewButton("Rename Selected", func() {
		toRename := s.getCheckedFiles()
		if len(toRename) == 0 {
			dialog.ShowInformation("No Files Selected", "Please check at least one file.", s.mainWindow)
			return
		}
		dialog.ShowEntryDialog(
			"Rename Selected Files",
			"Enter prefix for renamed files:",
			func(prefix string) {
				if prefix == "" {
					return
				}
				var renamedCount int
				var errs []string
				for _, fp := range toRename {
					dir := filepath.Dir(fp)
					ext := filepath.Ext(fp)
					oldName := filepath.Base(fp)
					newName := prefix + "_" + oldName + ext
					newPath := filepath.Join(dir, newName)
					err := os.Rename(fp, newPath)
					if err != nil {
						errs = append(errs, fmt.Sprintf("Failed to rename %s: %v", fp, err))
					} else {
						renamedCount++
					}
				}
				if len(errs) > 0 {
					dialog.ShowError(fmt.Errorf(strings.Join(errs, "\n")), s.mainWindow)
				}
				dialog.ShowInformation("Rename Complete", fmt.Sprintf("Renamed %d file(s).", renamedCount), s.mainWindow)
				s.refreshDuplicates()
			},
			s.mainWindow,
		)
	})

	sortLabel := widget.NewLabel("Sort By:")
	sortSelect := widget.NewSelect([]string{"Path", "Size"}, func(val string) {
		s.lastSelectedSort = val
		s.refreshDuplicates()
	})
	sortSelect.PlaceHolder = "(Select)"

	selectAllBtn := widget.NewButton("Select All", func() {
		for _, fi := range s.allFileItems {
			if fi.check != nil {
				fi.check.SetChecked(true)
			}
		}
	})
	deselectAllBtn := widget.NewButton("Deselect All", func() {
		for _, fi := range s.allFileItems {
			if fi.check != nil {
				fi.check.SetChecked(false)
			}
		}
	})

	// Pagination row for duplicates
	s.dfPageLabel = widget.NewLabel("")
	s.dfPrevBtn = widget.NewButton("<<< Prev Page", func() {
		if s.dfCurrentPage > 0 {
			s.dfCurrentPage--
			s.refreshDuplicates()
		}
	})
	s.dfNextBtn = widget.NewButton("Next Page >>>", func() {
		if s.dfCurrentPage < s.dfTotalPages-1 {
			s.dfCurrentPage++
			s.refreshDuplicates()
		}
	})
	dfPagingBox := container.NewHBox(s.dfPrevBtn, s.dfPageLabel, s.dfNextBtn)

	bottomBar := container.NewVBox(
		container.NewHBox(
			findDuplicatesBtn,
			deleteSelectedBtn,
			renameBtn,
			sortLabel,
			sortSelect,
			selectAllBtn,
			deselectAllBtn,
			layout.NewSpacer(),
		),
		dfPagingBox,
	)

	// Initialize so the label says "Page 0 of 0"
	s.dfTotalPages = 0
	s.updateDuplicatePageLabel()

	return container.NewBorder(
		topBar,
		bottomBar,
		nil,
		nil,
		scroll,
	)
}

// updateDuplicatePageLabel sets text & enables/disables next/prev
func (s *FileScanner) updateDuplicatePageLabel() {
	if s.dfTotalPages == 0 {
		s.dfPageLabel.SetText("Page 0 of 0")
		s.dfPrevBtn.Disable()
		s.dfNextBtn.Disable()
		return
	}
	s.dfPageLabel.SetText(fmt.Sprintf("Page %d of %d", s.dfCurrentPage+1, s.dfTotalPages))

	s.dfPrevBtn.Disable()
	s.dfNextBtn.Disable()

	if s.dfCurrentPage > 0 {
		s.dfPrevBtn.Enable()
	}
	if s.dfCurrentPage < s.dfTotalPages-1 {
		s.dfNextBtn.Enable()
	}
}

// refreshDuplicates shows only the current page of s.allFileItems
func (s *FileScanner) refreshDuplicates() {
	if s.duplicateListVBox == nil {
		return
	}
	s.duplicateListVBox.Objects = nil

	if len(s.allFileItems) == 0 {
		s.duplicateListVBox.Add(widget.NewLabel("No duplicate files found."))
		s.duplicateListVBox.Refresh()
		s.dfTotalPages = 1
		s.dfCurrentPage = 0
		s.updateDuplicatePageLabel()
		return
	}

	// Sort if needed
	if s.lastSelectedSort == "Size" {
		sort.Slice(s.allFileItems, func(i, j int) bool {
			return fileSize(s.allFileItems[i].filePath) < fileSize(s.allFileItems[j].filePath)
		})
	} else {
		sort.Slice(s.allFileItems, func(i, j int) bool {
			return s.allFileItems[i].filePath < s.allFileItems[j].filePath
		})
	}

	s.dfTotalPages = (len(s.allFileItems) + s.dfPageSize - 1) / s.dfPageSize
	if s.dfCurrentPage >= s.dfTotalPages {
		s.dfCurrentPage = s.dfTotalPages - 1
	}
	if s.dfCurrentPage < 0 {
		s.dfCurrentPage = 0
	}

	start := s.dfCurrentPage * s.dfPageSize
	end := start + s.dfPageSize
	if end > len(s.allFileItems) {
		end = len(s.allFileItems)
	}

	for i := start; i < end; i++ {
		fi := s.allFileItems[i]
		if fi.check == nil {
			fi.check = widget.NewCheck(fi.filePath, func(bool) {})
		}
		s.duplicateListVBox.Add(fi.check)
	}
	s.duplicateListVBox.Refresh()

	s.updateDuplicatePageLabel()
}

// getCheckedFiles returns file paths for the currently checked items in Duplicate Finder
func (s *FileScanner) getCheckedFiles() []string {
	var result []string
	for _, fi := range s.allFileItems {
		if fi.check != nil && fi.check.Checked {
			result = append(result, fi.filePath)
		}
	}
	return result
}

// helper to get file size
func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

// ---------------------------------------------------------------------
//  7) Space Cleaner UI
// ---------------------------------------------------------------------

func (s *FileScanner) setupSpaceCleanerUI() fyne.CanvasObject {
	spaceContainer := container.NewVBox()
	scroll := container.NewScroll(spaceContainer)
	scroll.SetMinSize(fyne.NewSize(0, 400))

	lbl := widget.NewLabel("Recommended Directories")

	chkDownloads := widget.NewCheck(RECOMM_DIR_DOWNLOADS, nil)
	chkTemp := widget.NewCheck(RECOMM_DIR_TEMP, nil)
	chkPrograms := widget.NewCheck(RECOMM_DIR_PROGRAMS, nil)
	chkAppData := widget.NewCheck(RECOMM_DIR_APPDATA, nil)

	selectAllRecs := widget.NewButton("Select All Recommended", func() {
		chkDownloads.SetChecked(true)
		chkTemp.SetChecked(true)
		chkPrograms.SetChecked(true)
		chkAppData.SetChecked(true)
	})

	scanRecsBtn := widget.NewButton("Scan", func() {
		spaceContainer.Objects = nil
		s.largeFileItems = []*LargeFileItem{}
		s.scCurrentPage = 0

		var dirs []string
		if chkDownloads.Checked {
			dirs = append(dirs, RECOMM_DIR_DOWNLOADS)
		}
		if chkTemp.Checked {
			dirs = append(dirs, RECOMM_DIR_TEMP)
		}
		if chkPrograms.Checked {
			dirs = append(dirs, RECOMM_DIR_PROGRAMS)
		}
		if chkAppData.Checked {
			dirs = append(dirs, RECOMM_DIR_APPDATA)
		}
		if len(dirs) == 0 {
			dialog.ShowInformation("No Directory", "No recommended directories selected.", s.mainWindow)
			return
		}
		s.showScanningLargeFiles(dirs, spaceContainer)
	})

	manualScanBtn := widget.NewButton("Manual Selection", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			fullURI := uri.String()
			if strings.HasPrefix(fullURI, "file://") {
				fullURI = strings.TrimPrefix(fullURI, "file://")
			}
			spaceContainer.Objects = nil
			s.largeFileItems = []*LargeFileItem{}
			s.scCurrentPage = 0
			s.showScanningLargeFiles([]string{fullURI}, spaceContainer)
		}, s.mainWindow)
	})

	purgeBtn := widget.NewButton("Purge Selected", func() {
		if len(s.largeFileItems) == 0 {
			dialog.ShowInformation("No Files", "Please scan first, then select files.", s.mainWindow)
			return
		}
		start := s.scCurrentPage * s.scPageSize
		end := start + s.scPageSize
		if end > len(s.largeFileItems) {
			end = len(s.largeFileItems)
		}

		var toDelete []string
		for i := start; i < end; i++ {
			lf := s.largeFileItems[i]
			if lf.check != nil && lf.check.Checked {
				toDelete = append(toDelete, lf.filePath)
			}
		}
		if len(toDelete) == 0 {
			dialog.ShowInformation("No Files Selected", "Select at least one file.", s.mainWindow)
			return
		}
		dialog.ShowConfirm("Confirm Purge", fmt.Sprintf("Purge %d file(s)?", len(toDelete)), func(c bool) {
			if !c {
				return
			}
			var errs []string
			var purged int
			for _, fp := range toDelete {
				err := os.Remove(fp)
				if err != nil {
					errs = append(errs, fmt.Sprintf("Failed to purge %s: %v", fp, err))
				} else {
					purged++
					s.addDeletionRecord(fp, "Space Cleaner")
				}
			}
			if len(errs) > 0 {
				dialog.ShowError(fmt.Errorf(strings.Join(errs, "\n")), s.mainWindow)
			}
			dialog.ShowInformation("Purge Complete", fmt.Sprintf("Purged %d file(s).", purged), s.mainWindow)
			s.refreshDeletionTable()

			// remove them from largeFileItems
			var newList []*LargeFileItem
			for _, lf := range s.largeFileItems {
				keep := true
				for _, d := range toDelete {
					if lf.filePath == d {
						keep = false
						break
					}
				}
				if keep {
					newList = append(newList, lf)
				}
			}
			s.largeFileItems = newList

			// recalc pages
			if len(s.largeFileItems) == 0 {
				s.scTotalPages = 1
			} else {
				s.scTotalPages = (len(s.largeFileItems) + s.scPageSize - 1) / s.scPageSize
			}
			if s.scCurrentPage >= s.scTotalPages {
				s.scCurrentPage = s.scTotalPages - 1
			}
			s.refreshLargeFiles(spaceContainer)
		}, s.mainWindow)
	})

	// Pagination row for space cleaner
	s.scPageLabel = widget.NewLabel("")
	s.scPrevBtn = widget.NewButton("<<< Prev Page", func() {
		if s.scCurrentPage > 0 {
			s.scCurrentPage--
			s.refreshLargeFiles(spaceContainer)
		}
	})
	s.scNextBtn = widget.NewButton("Next Page >>>", func() {
		if s.scCurrentPage < s.scTotalPages-1 {
			s.scCurrentPage++
			s.refreshLargeFiles(spaceContainer)
		}
	})
	scPagingBox := container.NewHBox(s.scPrevBtn, s.scPageLabel, s.scNextBtn)

	topBox := container.NewVBox(
		lbl,
		chkDownloads,
		chkTemp,
		chkPrograms,
		chkAppData,
		selectAllRecs,
		container.NewHBox(scanRecsBtn, manualScanBtn),
		scPagingBox,
	)

	bottomBox := container.NewHBox(
		purgeBtn,
		layout.NewSpacer(),
	)

	s.scTotalPages = 0
	s.updateSpaceCleanerPageLabel()

	return container.NewBorder(
		topBox,
		bottomBox,
		nil,
		nil,
		scroll,
	)
}

func (s *FileScanner) updateSpaceCleanerPageLabel() {
	if s.scTotalPages == 0 {
		s.scPageLabel.SetText("Page 0 of 0")
		s.scPrevBtn.Disable()
		s.scNextBtn.Disable()
		return
	}
	s.scPageLabel.SetText(fmt.Sprintf("Page %d of %d", s.scCurrentPage+1, s.scTotalPages))

	s.scPrevBtn.Disable()
	s.scNextBtn.Disable()

	if s.scCurrentPage > 0 {
		s.scPrevBtn.Enable()
	}
	if s.scCurrentPage < s.scTotalPages-1 {
		s.scNextBtn.Enable()
	}
}

// refreshLargeFiles shows only the current page
func (s *FileScanner) refreshLargeFiles(containerToFill *fyne.Container) {
	containerToFill.Objects = nil

	if len(s.largeFileItems) == 0 {
		containerToFill.Add(widget.NewLabel("No large files found."))
		containerToFill.Refresh()
		s.scTotalPages = 1
		s.scCurrentPage = 0
		s.updateSpaceCleanerPageLabel()
		return
	}

	s.scTotalPages = (len(s.largeFileItems) + s.scPageSize - 1) / s.scPageSize
	if s.scCurrentPage >= s.scTotalPages {
		s.scCurrentPage = s.scTotalPages - 1
	}
	if s.scCurrentPage < 0 {
		s.scCurrentPage = 0
	}

	start := s.scCurrentPage * s.scPageSize
	end := start + s.scPageSize
	if end > len(s.largeFileItems) {
		end = len(s.largeFileItems)
	}
	for i := start; i < end; i++ {
		lf := s.largeFileItems[i]
		if lf.check == nil {
			lf.check = widget.NewCheck(
				fmt.Sprintf("%s (%.2f MB)", lf.filePath, float64(lf.size)/1048576),
				func(bool) {},
			)
		}
		containerToFill.Add(lf.check)
	}
	containerToFill.Refresh()

	s.updateSpaceCleanerPageLabel()
}

// ---------------------------------------------------------------------
//  8) Deletion History (Full-Height Table)
// ---------------------------------------------------------------------

func (s *FileScanner) setupHistoryUI() fyne.CanvasObject {
	// We'll place "Save History" & "Clear" at the top, then the table in the center filling the window
	saveBtn := widget.NewButton("Save History", func() {
		if len(s.deletionRecords) == 0 {
			dialog.ShowInformation("No History", "No deleted files.", s.mainWindow)
			return
		}
		dialog.ShowFileSave(func(write fyne.URIWriteCloser, e error) {
			if e != nil || write == nil {
				return
			}
			defer write.Close()
			var sb strings.Builder
			for _, r := range s.deletionRecords {
				sb.WriteString(fmt.Sprintf("%s\t%s\t%s\n", r.Timestamp, r.FilePath, r.Method))
			}
			write.Write([]byte(sb.String()))
			dialog.ShowInformation("Saved", "Deletion history saved.", s.mainWindow)
		}, s.mainWindow)
	})

	clearBtn := widget.NewButton("Clear History", func() {
		s.deletionRecords = nil
		s.refreshDeletionTable()
	})

	topBar := container.NewHBox(
		saveBtn,
		clearBtn,
		layout.NewSpacer(),
	)

	table := s.makeHistoryTable()
	scrolled := container.NewScroll(table)      // wrap in a scroll so it can fill
	scrolled.SetMinSize(fyne.NewSize(900, 600)) // large area
	// Instead of just a small sliver, we let the table occupy entire center
	root := container.NewBorder(
		topBar,
		nil,
		nil,
		nil,
		scrolled,
	)
	return root
}

func (s *FileScanner) makeHistoryTable() fyne.CanvasObject {
	table := widget.NewTable(
		func() (int, int) {
			// #rows = #records + 1 (header), #cols=3
			return len(s.deletionRecords) + 1, 3
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			row, col := id.Row, id.Col
			label := obj.(*widget.Label)
			if row == 0 {
				// header
				switch col {
				case 0:
					label.SetText("Date/Time")
					label.TextStyle = fyne.TextStyle{Bold: true}
				case 1:
					label.SetText("File Path")
					label.TextStyle = fyne.TextStyle{Bold: true}
				case 2:
					label.SetText("Method")
					label.TextStyle = fyne.TextStyle{Bold: true}
				}
				return
			}
			recIndex := row - 1
			if recIndex < 0 || recIndex >= len(s.deletionRecords) {
				label.SetText("")
				return
			}
			rec := s.deletionRecords[recIndex]
			label.TextStyle = fyne.TextStyle{}
			switch col {
			case 0:
				label.SetText(rec.Timestamp)
			case 1:
				label.SetText(rec.FilePath)
			case 2:
				label.SetText(rec.Method)
			}
		},
	)
	table.SetColumnWidth(0, 180)
	table.SetColumnWidth(1, 550)
	table.SetColumnWidth(2, 150)

	return table
}

// After we modify s.deletionRecords (like clearing or adding new ones),
func (s *FileScanner) refreshDeletionTable() {
	if s.historyRoot == nil {
		return
	}
	newUI := s.setupHistoryUI()
	s.historyRoot = newUI
	if s.split != nil {
		// If we are currently on the "Deletion History" tab, re-show it
		s.switchRightContent(s.historyRoot)
	}
}

// ---------------------------------------------------------------------
//  9) Worker Functions
// ---------------------------------------------------------------------

func (s *FileScanner) addDeletionRecord(filePath, method string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	s.deletionRecords = append(s.deletionRecords, DeletionRecord{
		Timestamp: ts,
		FilePath:  filePath,
		Method:    method,
	})
}

func (s *FileScanner) showScanningDuplicates(dirPath, extFilter string) {
	pb := widget.NewProgressBarInfinite()
	lbl := widget.NewLabel("Scanning for duplicates...")
	vbox := container.NewVBox(lbl, pb)

	dlg := dialog.NewCustom("Please Wait", "Cancel", vbox, s.mainWindow)
	dlg.Show()

	go func() {
		files, e := s.scanDirectory(dirPath, extFilter)
		if e != nil {
			dlg.Hide()
			dialog.ShowError(e, s.mainWindow)
			return
		}
		// find duplicates
		m := s.findDuplicates(files)
		// flatten them into s.allFileItems
		var items []*FileItem
		for _, group := range m {
			if len(group) > 1 {
				for _, gpath := range group {
					items = append(items, &FileItem{filePath: gpath})
				}
			}
		}
		s.allFileItems = items
		dlg.Hide()

		if len(s.allFileItems) == 0 {
			dialog.ShowInformation("No Duplicates", "No duplicate files found.", s.mainWindow)
		} else {
			msg := fmt.Sprintf("Found %d total duplicate files.", len(s.allFileItems))
			dialog.ShowInformation("Scan Complete", msg, s.mainWindow)
		}
		s.dfCurrentPage = 0
		s.refreshDuplicates()
	}()
}

func loadPasswords() map[string]string {
	file, err := os.Open(passwordFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string)
		}
		fmt.Println("Error opening file:", err)
		os.Exit(1)
	}
	defer file.Close()

	encryptedPasswords := make(map[string]string)
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&encryptedPasswords); err != nil {
		fmt.Println("Error decoding JSON:", err)
		os.Exit(1)
	}

	passwords := make(map[string]string)
	for website, encrypted := range encryptedPasswords {
		decrypted, err := decrypt(encrypted)
		if err != nil {
			fmt.Println("Error decrypting password for", website, ":", err)
			os.Exit(1)
		}
		passwords[website] = decrypted
	}

	return passwords
}

func savePasswords(passwords map[string]string) {
	encryptedPasswords := make(map[string]string)
	for website, password := range passwords {
		encrypted, err := encrypt(password)
		if err != nil {
			fmt.Println("Error encrypting password for", website, ":", err)
			os.Exit(1)
		}
		encryptedPasswords[website] = encrypted
	}

	file, err := os.Create(passwordFilePath)
	if err != nil {
		fmt.Println("Error creating file:", err)
		os.Exit(1)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(&encryptedPasswords); err != nil {
		fmt.Println("Error encoding JSON:", err)
		os.Exit(1)
	}
}

func (s *FileScanner) showScanningLargeFiles(dirs []string, containerToFill *fyne.Container) {
	pb := widget.NewProgressBarInfinite()
	lbl := widget.NewLabel("Scanning for large files...")
	vbox := container.NewVBox(lbl, pb)

	dlg := dialog.NewCustom("Please Wait", "Cancel", vbox, s.mainWindow)
	dlg.Show()

	go func() {
		var allFiles []string
		for _, d := range dirs {
			fs, _ := s.scanDirectory(d, "")
			allFiles = append(allFiles, fs...)
		}
		sort.Slice(allFiles, func(i, j int) bool {
			si, _ := os.Stat(allFiles[i])
			sj, _ := os.Stat(allFiles[j])
			if si == nil || sj == nil {
				return false
			}
			return si.Size() > sj.Size()
		})

		for _, f := range allFiles {
			st, ee := os.Stat(f)
			if ee == nil && !st.IsDir() {
				lf := &LargeFileItem{filePath: f, size: st.Size()}
				s.largeFileItems = append(s.largeFileItems, lf)
			}
		}
		dlg.Hide()

		if len(s.largeFileItems) == 0 {
			dialog.ShowInformation("No Files", "No large files found.", s.mainWindow)
		}
		s.scCurrentPage = 0
		s.refreshLargeFiles(containerToFill)
	}()
}

func encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher([]byte(encryptionKey))
	if err != nil {
		return "", err
	}

	nonce := make([]byte, 12)
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	ciphertext := aesgcm.Seal(nil, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(append(nonce, ciphertext...)), nil
}

func decrypt(encryptedText string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encryptedText)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher([]byte(encryptionKey))
	if err != nil {
		return "", err
	}

	if len(data) < 12 {
		return "", fmt.Errorf("invalid ciphertext")
	}

	nonce, ciphertext := data[:12], data[12:]
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func (s *FileScanner) scanDirectory(dirPath, extFilter string) ([]string, error) {
	var files []string
	filterSet := make(map[string]bool)

	if extFilter != "" {
		parts := strings.Split(extFilter, ",")
		for _, part := range parts {
			trim := strings.ToLower(strings.TrimSpace(part))
			if trim != "" {
				filterSet[trim] = true
			}
		}
	}

	var mu sync.Mutex
	err := filepath.Walk(dirPath, func(p string, info os.FileInfo, wErr error) error {
		if wErr != nil {
			return wErr
		}
		if !info.IsDir() {
			if len(filterSet) > 0 {
				ext := strings.ToLower(filepath.Ext(p))
				if !filterSet[ext] {
					return nil
				}
			}
			mu.Lock()
			files = append(files, p)
			mu.Unlock()
		}
		return nil
	})
	return files, err
}

func (s *FileScanner) findDuplicates(fileList []string) map[string][]string {
	h := make(map[string][]string)
	for _, fp := range fileList {
		hashStr, e := s.generateHash(fp)
		if e != nil {
			continue
		}
		st, e2 := os.Stat(fp)
		if e2 != nil {
			continue
		}
		key := fmt.Sprintf("%s-%d", hashStr, st.Size())
		h[key] = append(h[key], fp)
	}
	// only keep groups of size > 1
	res := make(map[string][]string)
	for k, group := range h {
		if len(group) > 1 {
			res[k] = group
		}
	}
	return res
}

func (s *FileScanner) generateHash(filePath string) (string, error) {
	f, e := os.Open(filePath)
	if e != nil {
		return "", e
	}
	defer f.Close()

	st, e2 := f.Stat()
	if e2 != nil || st.Size() == 0 {
		return "", fmt.Errorf("file is unreadable or empty")
	}

	h := sha256.New()
	_, e2 = io.Copy(h, f)
	if e2 != nil {
		return "", e2
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s *FileScanner) summarize(d map[string][]string) (int, int64) {
	var c int
	var sz int64
	seen := make(map[string]bool)
	for _, group := range d {
		for _, fp := range group {
			if seen[fp] {
				continue
			}
			seen[fp] = true
			st, er := os.Stat(fp)
			if er == nil {
				c++
				sz += st.Size()
			}
		}
	}
	return c, sz
}

func (s *FileScanner) setupPasswordManagerUI() fyne.CanvasObject {
	passwords := loadPasswords()

	passwordList := widget.NewList(
		func() int {
			return len(passwords)
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			index := 0
			for website := range passwords {
				if index == id {
					obj.(*widget.Label).SetText(website)
					return
				}
				index++
			}
		},
	)

	addPasswordBtn := widget.NewButton("Add Password", func() {
		websiteEntry := widget.NewEntry()
		passwordEntry := widget.NewPasswordEntry()

		dialog.ShowForm("Add Password", "Save", "Cancel", []*widget.FormItem{
			widget.NewFormItem("Website", websiteEntry),
			widget.NewFormItem("Password", passwordEntry),
		}, func(confirm bool) {
			if confirm {
				website := strings.TrimSpace(websiteEntry.Text)
				password := strings.TrimSpace(passwordEntry.Text)

				if website == "" || password == "" {
					dialog.ShowInformation("Invalid Input", "Website and Password cannot be empty.", s.mainWindow)
					return
				}

				// Add to passwords map
				passwords[website] = password

				// Save to JSON file
				savePasswords(passwords)

				// Refresh password list
				passwordList.Refresh()

				dialog.ShowInformation("Success", "Password added successfully.", s.mainWindow)
			}
		}, s.mainWindow)
	})

	removePasswordBtn := widget.NewButton("Remove Password", func() {
		dialog.ShowEntryDialog("Remove Password", "Enter website to remove:", func(website string) {
			if _, exists := passwords[website]; exists {
				delete(passwords, website)
				savePasswords(passwords)
				passwordList.Refresh()
			} else {
				dialog.ShowInformation("Not Found", "No password found for "+website, s.mainWindow)
			}
		}, s.mainWindow)
	})

	viewPasswordBtn := widget.NewButton("View Password", func() {
		dialog.ShowEntryDialog("View Password", "Enter website to view:", func(website string) {
			if password, exists := passwords[website]; exists {
				dialog.ShowInformation("Password", fmt.Sprintf("Password for %s: %s", website, password), s.mainWindow)
			} else {
				dialog.ShowInformation("Not Found", "No password found for "+website, s.mainWindow)
			}
		}, s.mainWindow)
	})

	controls := container.NewHBox(
		addPasswordBtn,
		removePasswordBtn,
		viewPasswordBtn,
		layout.NewSpacer(),
	)

	return container.NewBorder(
		controls,
		nil,
		nil,
		nil,
		passwordList,
	)
}
