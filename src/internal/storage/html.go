package storage

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

const MaxHTMLBytes = 5 * 1024 * 1024

const (
	sourceURLPrefix         = "<!-- key-crawl-source-url-base64: "
	sourceURLSuffix         = " -->"
	maxSourceURLHeaderBytes = 64 * 1024
)

var (
	ErrSourceURLRequired = errors.New("来源 URL 为空")
	ErrSourceURLTooLong  = errors.New("来源 URL 过长")
	ErrHTMLTooLarge      = errors.New("HTML 超过 5 MiB 大小限制")
)

func ListHTML() ([]string, error) {
	entries, err := os.ReadDir(htmlFolder)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && path.Ext(entry.Name()) == ".html" {
			files = append(files, entry.Name())
		}
	}
	return files, nil
}

func SaveHTML(url string, body io.Reader, contentLength int64) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return ErrSourceURLRequired
	}
	// Standard Base64 contains no hyphens, so it cannot introduce "--" in a comment.
	header := sourceURLPrefix + base64.StdEncoding.EncodeToString([]byte(url)) + sourceURLSuffix + "\n"
	if len(header) > maxSourceURLHeaderBytes {
		return ErrSourceURLTooLong
	}
	if contentLength > MaxHTMLBytes {
		return ErrHTMLTooLarge
	}
	fileName := fmt.Sprintf("%x.html", sha256.Sum256([]byte(url)))
	f, err := os.CreateTemp(htmlFolder, ".upload-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.WriteString(header); err != nil {
		return err
	}
	written, err := f.ReadFrom(io.LimitReader(body, MaxHTMLBytes+1))
	if err != nil {
		return err
	}
	if written > MaxHTMLBytes {
		return ErrHTMLTooLarge
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path.Join(htmlFolder, fileName))
}

func ReadSavedHTML(fileName string) (string, string, error) {
	f, err := os.Open(path.Join(htmlFolder, fileName))
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", "", err
	}
	if info.Size() > MaxHTMLBytes+maxSourceURLHeaderBytes {
		return "", "", fmt.Errorf("HTML 超过 5 MiB 大小限制，已跳过")
	}
	reader := bufio.NewReaderSize(f, maxSourceURLHeaderBytes)
	headerBytes, err := reader.ReadSlice('\n')
	if err != nil {
		return "", "", fmt.Errorf("读取 HTML 来源注释失败（缺失或过长）: %w", err)
	}
	header := strings.TrimSuffix(strings.TrimSuffix(string(headerBytes), "\n"), "\r")
	if !strings.HasPrefix(header, sourceURLPrefix) || !strings.HasSuffix(header, sourceURLSuffix) {
		return "", "", fmt.Errorf("HTML 首行缺少有效的来源 URL 注释")
	}
	encodedURL := strings.TrimSuffix(strings.TrimPrefix(header, sourceURLPrefix), sourceURLSuffix)
	decodedURL, err := base64.StdEncoding.DecodeString(encodedURL)
	if err != nil {
		return "", "", fmt.Errorf("解码 HTML 来源 URL: %w", err)
	}
	url := string(decodedURL)
	if strings.TrimSpace(url) == "" {
		return "", "", fmt.Errorf("HTML 来源 URL 为空")
	}
	content, err := io.ReadAll(io.LimitReader(reader, MaxHTMLBytes+1))
	if err != nil {
		return "", "", err
	}
	if len(content) > MaxHTMLBytes {
		return "", "", fmt.Errorf("HTML 超过 5 MiB 大小限制，已跳过")
	}
	return url, string(content), nil
}

func RemoveHTML(fileName string) error {
	return os.Remove(path.Join(htmlFolder, fileName))
}
