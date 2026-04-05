//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghost-mcp/internal/logging"
	"github.com/ghost-mcp/internal/ocr"
)

// SetupWindowsEnv configures the Path and TESSDATA_PREFIX environment variables
// to allow the server to find Tesseract DLLs and data files when running within
// a vcpkg-managed MinGW environment.
func SetupWindowsEnv() {
	home, _ := os.UserHomeDir()

	// 1. Configure Path for DLLs
	vcpkgBin := filepath.Join(home, "vcpkg", "installed", "x64-mingw-dynamic", "bin")
	if _, err := os.Stat(vcpkgBin); err == nil {
		currentPath := os.Getenv("Path")
		if !strings.Contains(strings.ToLower(currentPath), strings.ToLower(vcpkgBin)) {
			logging.Info("Auto-configuring Windows environment: prepending vcpkg bin to Path: %s", vcpkgBin)
			os.Setenv("Path", fmt.Sprintf("%s;%s", vcpkgBin, currentPath))
		}
	}

	// 2. Configure TESSDATA_PREFIX
	vcpkgTessData := filepath.Join(home, "vcpkg", "installed", "x64-mingw-dynamic", "share", "tessdata")
	currentTessPrefix := os.Getenv("TESSDATA_PREFIX")

	// If already set, check if it's potentially wrong (e.g. missing 'tessdata' suffix)
	isWrong := currentTessPrefix != "" && !strings.HasSuffix(strings.ToLower(currentTessPrefix), "tessdata")

	if currentTessPrefix == "" || isWrong {
		engData := filepath.Join(vcpkgTessData, "eng.traineddata")
		if _, err := os.Stat(engData); err == nil {
			if isWrong {
				logging.Info("Auto-configuring Windows environment: overwriting broken TESSDATA_PREFIX (%s) with %s", currentTessPrefix, vcpkgTessData)
			} else {
				logging.Info("Auto-configuring Windows environment: setting TESSDATA_PREFIX to %s", vcpkgTessData)
			}
			os.Setenv("TESSDATA_PREFIX", vcpkgTessData)
			// Force sync to C runtime environment
			ocr.SyncTessDataPrefix()
		} else {
			logging.Error("setupWindowsEnv: Tesseract data not found at %s. Please ensure vcpkg x64-mingw-dynamic tesseract[core] is installed.", engData)
		}
	} else {
		logging.Debug("setupWindowsEnv: TESSDATA_PREFIX already set to %s", currentTessPrefix)
	}
}
