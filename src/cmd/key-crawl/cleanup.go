package main

import (
	"os"
	"path/filepath"
	"time"
)

func cleanupUpdateFiles(stagingDirectory string) {
	target, err := os.Executable()
	if err != nil || !isUpdateStagingDirectory(target, stagingDirectory) {
		return
	}
	executableBackup := target + ".old"
	extensionBackup := filepath.Join(filepath.Dir(target), "extension.old")
	for range 50 {
		_ = os.Remove(executableBackup)
		_ = os.RemoveAll(extensionBackup)
		_ = os.RemoveAll(filepath.Clean(stagingDirectory))
		if !pathExists(executableBackup) && !pathExists(extensionBackup) && !pathExists(stagingDirectory) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || !os.IsNotExist(err)
}
