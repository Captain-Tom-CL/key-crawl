package analysis

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/captain-tom-cl/key-crawl/src/internal/storage"
)

type ResultFile struct {
	AnalysisResult
	URL      string `json:"url"`
	FileName string `json:"fileName"`
}

func LoadResults() ([]ResultFile, []string, error) {
	files, err := storage.ListResults()
	if err != nil {
		return nil, nil, err
	}
	results := make([]ResultFile, 0, len(files))
	warnings := make([]string, 0)
	for _, fileName := range files {
		result, err := readResult(fileName)
		if err != nil {
			warnings = append(warnings, fileName+"："+err.Error())
			continue
		}
		results = append(results, result)
	}
	return results, warnings, nil
}

func readResult(fileName string) (ResultFile, error) {
	data, err := storage.ReadResult(fileName)
	if err != nil {
		return ResultFile{}, err
	}
	var result storedAnalysisResult
	if err := json.Unmarshal(data, &result); err != nil {
		return ResultFile{}, err
	}
	if strings.TrimSpace(result.URL) == "" {
		return ResultFile{}, fmt.Errorf("分析结果缺少来源 URL")
	}
	return ResultFile{
		AnalysisResult: result.AnalysisResult,
		URL:            result.URL,
		FileName:       fileName,
	}, nil
}
