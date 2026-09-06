package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const FilePath = "data/settings.json"

const (
	defaultAnalysisConcurrency = 8
	defaultRequestsPerMinute   = 60
	maxAnalysisConcurrency     = 32
	maxRequestsPerMinute       = 6000
)

var (
	mu      sync.RWMutex
	current = Default()
	loadErr error
)

type Config struct {
	ApiKey            string `json:"apiKey"`
	BaseURL           string `json:"baseURL"`
	Model             string `json:"model"`
	Prompt            string `json:"prompt"`
	Concurrency       int    `json:"concurrency"`
	RequestsPerMinute int    `json:"requestsPerMinute"`
}

func Default() *Config {
	return &Config{
		Concurrency:       defaultAnalysisConcurrency,
		RequestsPerMinute: defaultRequestsPerMinute,
	}
}

func (s Config) Validate() error {
	if s.Concurrency < 1 || s.Concurrency > maxAnalysisConcurrency {
		return fmt.Errorf("分析并发数必须在 1 到 %d 之间", maxAnalysisConcurrency)
	}
	if s.RequestsPerMinute < 0 || s.RequestsPerMinute > maxRequestsPerMinute {
		return fmt.Errorf("每分钟请求数必须在 0 到 %d 之间（0 表示不主动限速）", maxRequestsPerMinute)
	}
	return nil
}

func Initialize() error {
	mu.Lock()
	defer mu.Unlock()
	loaded, err := readFile()
	loadErr = err
	if err == nil {
		current = loaded
	} else {
		current = Default()
	}
	return err
}

func Snapshot() (Config, error) {
	mu.RLock()
	defer mu.RUnlock()
	return *current, loadErr
}

// Load reads a fresh configuration snapshot for the next analysis batch.
func Load() (*Config, error) {
	mu.RLock()
	defer mu.RUnlock()
	return readFile()
}

// readFile requires mu to be held by the caller.
func readFile() (*Config, error) {
	data, err := os.ReadFile(FilePath)
	if os.IsNotExist(err) {
		return Default(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取配置文件: %w", err)
	}
	config := Default()
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("解析配置文件: %w", err)
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("分析配置无效: %w", err)
	}
	return config, nil
}

func Save(candidate Config) error {
	data, err := json.Marshal(candidate)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()

	file, err := os.CreateTemp(filepath.Dir(FilePath), ".settings-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时配置文件: %w", err)
	}
	temporaryPath := file.Name()
	defer os.Remove(temporaryPath)
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("写入临时配置文件: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("同步临时配置文件: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("关闭临时配置文件: %w", err)
	}
	// Never truncate or remove the current file before replacement. On Windows,
	// Rename is not guaranteed atomic; mu also protects in-process readers.
	if err := os.Rename(temporaryPath, FilePath); err != nil {
		return fmt.Errorf("替换配置文件: %w", err)
	}
	current = &candidate
	loadErr = nil
	return nil
}
