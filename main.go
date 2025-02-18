package main

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
	"golang.org/x/sys/windows/registry"
)

const (
	registryPath = `FiveM.ProtocolHandler\shell\open\command`
	appTitle     = "FiveM Cache Cleaner"
	windowWidth  = 500
	windowHeight = 400
	buttonWidth  = 200
	buttonHeight = 40
	logHeight    = 300
	MB_YESNO     = 0x4
	IDYES        = 6
)

var (
	user32     = syscall.NewLazyDLL("user32.dll")
	messageBox = user32.NewProc("MessageBoxW")
)
var FivemExec = ""

var cacheFolders = []string{
	"cache",
	"server-cache",
	"server-cache-priv",
}

//go:generate goversioninfo -icon=app.ico -manifest=app.manifest
//go:embed app.ico
var appIcon embed.FS

type FiveMCacheCleaner struct {
	sync.Mutex
	window      *walk.MainWindow
	logOutput   *walk.TextEdit
	paths       []string
	cleanButton *walk.PushButton
	isRunning   bool
}

func NewFiveMCacheCleaner() *FiveMCacheCleaner {
	return &FiveMCacheCleaner{
		paths: make([]string, 0, 1),
	}
}

func (f *FiveMCacheCleaner) log(message string) {
	if f.logOutput != nil {
		f.logOutput.Synchronize(func() {
			f.logOutput.AppendText(message + "\r\n")
		})
	}
}

func (f *FiveMCacheCleaner) detectFiveMPath() error {
	f.Lock()
	defer f.Unlock()

	f.paths = f.paths[:0]
	f.log("Looking for FiveM installation...")

	key, err := registry.OpenKey(registry.CLASSES_ROOT, registryPath, registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("error opening registry key: %v", err)
	}
	defer key.Close()

	value, _, err := key.GetStringValue("")
	if err != nil {
		return fmt.Errorf("error reading registry value: %v", err)
	}

	path := strings.Trim(strings.Split(value, "\" ")[0], "\"")
	path = strings.TrimSuffix(path, "FiveM.exe")
	path = filepath.Join(path, "FiveM.app")
	FivemExec = strings.Trim(strings.Split(value, "\" ")[0], "\"")
	if f.isFiveMInstallation(path) {
		f.paths = append(f.paths, path)
		f.log(fmt.Sprintf("✓ Found FiveM installation at: %s", path))
		return nil
	}

	return fmt.Errorf("no valid FiveM installation found")
}

func (f *FiveMCacheCleaner) isFiveMInstallation(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(path, "data"))
	return err == nil
}

func (f *FiveMCacheCleaner) cleanCache() {
	f.Lock()
	if f.isRunning {
		f.Unlock()
		return
	}
	f.isRunning = true
	f.Unlock()

	defer func() {
		f.Lock()
		f.isRunning = false
		f.Unlock()
	}()

	f.updateButtonState(false, "Cleaning...")
	f.clearLog()

	if err := f.detectFiveMPath(); err != nil {
		f.log(fmt.Sprintf("❌ %v", err))
		f.updateButtonState(true, "Clean FiveM Cache")
		return
	}

	f.cleanCacheFolders()
	f.updateButtonState(true, "Clean FiveM Cache")
}

func (f *FiveMCacheCleaner) cleanCacheFolders() {
	f.Lock()
	paths := make([]string, len(f.paths))
	copy(paths, f.paths)
	f.Unlock()

	for _, path := range paths {
		dataPath := filepath.Join(path, "data")
		f.log("\nCleaning cache folders:")

		for _, folder := range cacheFolders {
			fullPath := filepath.Join(dataPath, folder)
			if err := os.RemoveAll(fullPath); err != nil {
				f.log(fmt.Sprintf("❌ Error removing %s: %v", folder, err))
			} else {
				f.log(fmt.Sprintf("✓ Removed %s", folder))
			}
		}
	}

	f.log("\n✨ Cache cleaning completed successfully!")
	ret, _, _ := messageBox.Call(
		0,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("Do you want to run FiveM?"))),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("Cache finished!"))),
		uintptr(MB_YESNO))

	if int(ret) == IDYES {

		err := exec.Command(FivemExec).Start()
		if err != nil {
			panic(err)
			return
		}
		os.Exit(0)
	} else {
		os.Exit(-1)
	}
}

func (f *FiveMCacheCleaner) updateButtonState(enabled bool, text string) {
	f.cleanButton.Synchronize(func() {
		f.cleanButton.SetEnabled(enabled)
		f.cleanButton.SetText(text)
	})
}

func (f *FiveMCacheCleaner) clearLog() {
	f.logOutput.Synchronize(func() {
		f.logOutput.SetText("")
	})
}

func (f *FiveMCacheCleaner) createMainWindow() error {
	var mainWindow *walk.MainWindow
	var icon *walk.Icon

	tempDir, err := os.MkdirTemp("", "app_icon")
	if err != nil {
		return fmt.Errorf("error creating temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	iconPath := filepath.Join(tempDir, "app.ico")
	iconData, err := appIcon.ReadFile("app.ico")
	if err != nil {
		return fmt.Errorf("error reading embedded icon: %v", err)
	}
	if err := os.WriteFile(iconPath, iconData, 0644); err != nil {
		return fmt.Errorf("error writing icon file: %v", err)
	}

	icon, err = walk.NewIconFromFile(iconPath)
	if err != nil {
		return fmt.Errorf("error loading icon: %v", err)
	}
	defer icon.Dispose()
	err = MainWindow{
		AssignTo: &mainWindow,
		Title:    appTitle,
		Size:     Size{Width: windowWidth, Height: windowHeight},
		Layout:   VBox{},
		Children: []Widget{
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					HSpacer{},
					PushButton{
						AssignTo: &f.cleanButton,
						Text:     "Clean FiveM Cache",
						MinSize:  Size{Width: buttonWidth, Height: buttonHeight},
						Font:     Font{Family: "Segoe UI", PointSize: 10, Bold: true},
						OnClicked: func() {
							go f.cleanCache()
						},
					},
					HSpacer{},
				},
			},
			Composite{
				Layout: VBox{},
				Children: []Widget{
					TextEdit{
						AssignTo:  &f.logOutput,
						ReadOnly:  true,
						VScroll:   true,
						Font:      Font{Family: "Consolas", PointSize: 9},
						MinSize:   Size{Height: logHeight},
						TextColor: walk.RGB(33, 33, 33),
					},
				},
			},
		},
	}.Create()

	if err != nil {
		return fmt.Errorf("error creating main window: %v", err)
	}

	style := win.GetWindowLong(mainWindow.Handle(), win.GWL_STYLE)
	style &^= win.WS_MAXIMIZEBOX | win.WS_MINIMIZEBOX
	win.SetWindowLong(mainWindow.Handle(), win.GWL_STYLE, style)

	f.window = mainWindow

	iconPathPtr, err := syscall.UTF16PtrFromString(iconPath)
	if err != nil {
		return fmt.Errorf("error converting icon path: %v", err)
	}

	hIcon := win.LoadImage(
		0,
		iconPathPtr,
		win.IMAGE_ICON,
		0,
		0,
		win.LR_LOADFROMFILE|win.LR_DEFAULTSIZE,
	)

	if hIcon != 0 {
		win.SendMessage(mainWindow.Handle(), win.WM_SETICON, 1, uintptr(hIcon))
		win.SendMessage(mainWindow.Handle(), win.WM_SETICON, 0, uintptr(hIcon))
	}
	return nil
}

func main() {
	updateManager := NewUpdateManager("owner", "repo")

	if updateManager.CheckVersion("v1.0.0.6") {
		updateManager.DownloadLatestAsset("app.exe", "v1.0.0.6")
	}

	cleaner := NewFiveMCacheCleaner()
	if err := cleaner.createMainWindow(); err != nil {
		fmt.Printf("Error creating main window: %v\n", err)
		os.Exit(1)
	}
	cleaner.window.Run()
}
