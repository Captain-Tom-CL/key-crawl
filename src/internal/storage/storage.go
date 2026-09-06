package storage

import "os"

const (
	htmlFolder    = "data/html"
	resultsFolder = "data/results"
)

func Initialize() error {
	if err := os.MkdirAll(htmlFolder, 0755); err != nil {
		return err
	}
	return os.MkdirAll(resultsFolder, 0755)
}
