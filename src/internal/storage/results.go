package storage

import (
	"os"
	"path/filepath"
	"strings"
)

func ListResults() ([]string, error) {
	// os.ReadDir returns entries sorted by filename.
	entries, err := os.ReadDir(resultsFolder)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			files = append(files, entry.Name())
		}
	}
	return files, nil
}

func ReadResult(fileName string) ([]byte, error) {
	return os.ReadFile(filepath.Join(resultsFolder, fileName))
}

func WriteResult(fileName string, data []byte) error {
	return os.WriteFile(filepath.Join(resultsFolder, fileName), data, 0644)
}
