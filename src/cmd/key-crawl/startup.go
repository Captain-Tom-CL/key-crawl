package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func setApplicationWorkingDirectory() error {
	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取程序路径: %w", err)
	}
	rootDirectory := filepath.Dir(executablePath)
	if err := os.Chdir(rootDirectory); err != nil {
		return fmt.Errorf("切换到 %s: %w", rootDirectory, err)
	}
	return nil
}
