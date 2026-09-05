package main

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	latestReleaseURL     = "https://api.github.com/repos/Captain-Tom-CL/key-crawl/releases/latest"
	executableName       = "key-crawl.exe"
	extensionAssetName   = "extension.zip"
	extensionVersionName = "extension-version.txt"
	maxAssetSize         = 200 << 20
)

var (
	updateCheckClient    = &http.Client{Timeout: 8 * time.Second}
	updateDownloadClient = &http.Client{Timeout: 5 * time.Minute}
	markdownLinkPattern  = regexp.MustCompile(`\[([^]]+)]\(https?://[^)[:space:]]+\)`)
	webURLPattern        = regexp.MustCompile(`https?://[^[:space:]<>)]+`)
	sourceNamePattern    = regexp.MustCompile(`(?i)github(?:\.com)?`)
)

type releaseAsset struct {
	Name   string `json:"name"`
	APIURL string `json:"url"`
	Digest string `json:"digest"`
}

type latestRelease struct {
	Name    string         `json:"name"`
	TagName string         `json:"tag_name"`
	Body    string         `json:"body"`
	Assets  []releaseAsset `json:"assets"`
}

func update() (bool, error) {
	fmt.Println("正在检查更新...")
	executablePath, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("获取程序路径: %w", err)
	}
	executablePath, err = filepath.Abs(executablePath)
	if err != nil {
		return false, fmt.Errorf("解析程序路径: %w", err)
	}
	rootDirectory := filepath.Dir(executablePath)
	extensionDirectory := filepath.Join(rootDirectory, "extension")
	release, err := fetchLatestRelease()
	if err != nil {
		return false, err
	}
	releaseAssets := make(map[string]releaseAsset, len(release.Assets))
	for _, asset := range release.Assets {
		releaseAssets[asset.Name] = asset
	}
	executableAsset, err := requireReleaseAsset(releaseAssets, executableName, true)
	if err != nil {
		return false, err
	}
	extensionAsset, err := requireReleaseAsset(releaseAssets, extensionAssetName, true)
	if err != nil {
		return false, err
	}
	extensionVersionAsset, err := requireReleaseAsset(releaseAssets, extensionVersionName, false)
	if err != nil {
		return false, err
	}

	localExecutableChecksum, err := fileSHA256(executablePath)
	if err != nil {
		return false, fmt.Errorf("计算当前程序 SHA-256: %w", err)
	}
	remoteExecutableChecksum := strings.TrimPrefix(strings.ToLower(executableAsset.Digest), "sha256:")
	remoteExtensionVersion, err := fetchAssetText(extensionVersionAsset.APIURL, 64)
	if err != nil {
		return false, err
	}
	if remoteExtensionVersion == "" {
		return false, errors.New("远端扩展版本为空")
	}
	localExtensionVersion, extensionErr := readExtensionVersion(extensionDirectory)
	extensionMissing := errors.Is(extensionErr, os.ErrNotExist)
	if extensionErr != nil && !extensionMissing {
		return false, fmt.Errorf("读取本地扩展版本: %w", extensionErr)
	}
	executableChanged := !strings.EqualFold(localExecutableChecksum, remoteExecutableChecksum)
	extensionChanged := extensionMissing || localExtensionVersion != remoteExtensionVersion
	if !executableChanged && !extensionChanged {
		fmt.Println("当前已是最新版本。")
		return false, nil
	}

	if extensionChanged {
		if extensionMissing {
			fmt.Println("未找到完整的 extension 目录，可以下载并恢复。")
		} else {
			fmt.Printf("扩展版本不一致（本地 %s，远端 %s）。\n", localExtensionVersion, remoteExtensionVersion)
		}
	}
	if executableChanged {
		fmt.Println("发现程序更新。")
	}

	printReleaseNotes(release)
	fmt.Print("是否下载并安装本次更新？[y/N]: ")
	input, readErr := bufio.NewReader(os.Stdin).ReadString('\n')
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return false, fmt.Errorf("读取更新选择: %w", readErr)
	}
	if answer := strings.ToLower(strings.TrimSpace(input)); answer != "y" && answer != "yes" {
		fmt.Println("已跳过本次更新。")
		return false, nil
	}

	stagingDirectory, err := os.MkdirTemp(filepath.Dir(executablePath), ".key-crawl-update-")
	if err != nil {
		return false, fmt.Errorf("创建更新暂存目录: %w", err)
	}
	keepStaging := false
	defer func() {
		if !keepStaging {
			_ = os.RemoveAll(stagingDirectory)
		}
	}()

	stagedExecutable := filepath.Join(stagingDirectory, executableName)
	stagedArchive := filepath.Join(stagingDirectory, extensionAssetName)
	stagedExtension := filepath.Join(stagingDirectory, "extension")
	assets := []struct {
		name     string
		url      string
		path     string
		checksum string
	}{
		{name: executableName, url: executableAsset.APIURL, path: stagedExecutable, checksum: remoteExecutableChecksum},
		{name: extensionAssetName, url: extensionAsset.APIURL, path: stagedArchive, checksum: strings.TrimPrefix(strings.ToLower(extensionAsset.Digest), "sha256:")},
	}
	for _, asset := range assets {
		fmt.Printf("正在下载 %s...\n", asset.name)
		if downloadErr := downloadAndVerify(asset.url, asset.path, asset.checksum); downloadErr != nil {
			return false, downloadErr
		}
	}
	if err := extractZip(stagedArchive, stagedExtension); err != nil {
		return false, fmt.Errorf("解压扩展更新: %w", err)
	}
	if err := os.Remove(stagedArchive); err != nil {
		return false, fmt.Errorf("删除扩展压缩包: %w", err)
	}
	stagedExtensionVersion, err := readExtensionVersion(stagedExtension)
	if err != nil {
		return false, fmt.Errorf("读取下载扩展版本: %w", err)
	}
	if stagedExtensionVersion != remoteExtensionVersion {
		return false, fmt.Errorf("下载扩展版本 %q 与远端版本 %q 不一致", stagedExtensionVersion, remoteExtensionVersion)
	}

	process, err := os.StartProcess(stagedExecutable, []string{
		stagedExecutable,
		"-apply-update", executablePath,
		"-extension-update", extensionDirectory,
	}, &os.ProcAttr{
		Dir:   rootDirectory,
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	})
	if err != nil {
		return false, fmt.Errorf("启动更新助手: %w", err)
	}
	_ = process.Release()
	keepStaging = true
	fmt.Println("更新已下载，正在重启应用...")
	return true, nil
}

func fetchLatestRelease() (latestRelease, error) {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return latestRelease{}, errors.New("创建更新检查请求失败")
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "key-crawl-updater")
	response, err := updateCheckClient.Do(request)
	if err != nil {
		return latestRelease{}, errors.New("连接更新服务失败")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return latestRelease{}, fmt.Errorf("更新服务返回 HTTP %d", response.StatusCode)
	}
	var release latestRelease
	decoder := json.NewDecoder(io.LimitReader(response.Body, 2<<20))
	if err := decoder.Decode(&release); err != nil {
		return latestRelease{}, errors.New("解析更新信息失败")
	}
	return release, nil
}

func printReleaseNotes(release latestRelease) {
	title := sanitizeReleaseText(release.Name)
	if title == "" {
		title = sanitizeReleaseText(release.TagName)
	} else if tag := sanitizeReleaseText(release.TagName); tag != "" && tag != title {
		title += " (" + tag + ")"
	}
	if title == "" {
		title = "最新版本"
	}

	notes := sanitizeReleaseText(release.Body)
	if notes == "" {
		notes = "本次发布未提供更新说明。"
	}

	fmt.Println()
	fmt.Println("\033[1;36m━━━━━━━━━━━━━━━━━━ 更新说明 ━━━━━━━━━━━━━━━━━━\033[0m")
	fmt.Printf("\033[1;37m%s\033[0m\n\n%s\n", title, notes)
	fmt.Println("\033[1;36m━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\033[0m")
	fmt.Println()
}

func sanitizeReleaseText(value string) string {
	value = markdownLinkPattern.ReplaceAllString(value, "$1")
	value = webURLPattern.ReplaceAllString(value, "")
	value = sourceNamePattern.ReplaceAllString(value, "代码托管平台")
	return strings.TrimSpace(value)
}

func requireReleaseAsset(assets map[string]releaseAsset, name string, requireDigest bool) (releaseAsset, error) {
	asset, ok := assets[name]
	if !ok || asset.APIURL == "" {
		return releaseAsset{}, fmt.Errorf("更新包缺少文件 %s", name)
	}
	if requireDigest {
		normalizedDigest := strings.ToLower(asset.Digest)
		if !strings.HasPrefix(normalizedDigest, "sha256:") {
			return releaseAsset{}, fmt.Errorf("更新文件 %s 未提供 SHA-256 digest", name)
		}
		digest := strings.TrimPrefix(normalizedDigest, "sha256:")
		if len(digest) != 64 {
			return releaseAsset{}, fmt.Errorf("更新文件 %s 缺少有效的 SHA-256 digest", name)
		}
		if _, err := hex.DecodeString(digest); err != nil {
			return releaseAsset{}, fmt.Errorf("更新文件 %s 的 digest 无效", name)
		}
	}
	return asset, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func readExtensionVersion(extensionDirectory string) (string, error) {
	manifestPath := filepath.Join(extensionDirectory, "manifest.json")
	file, err := os.Open(manifestPath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	var manifest struct {
		Version string `json:"version"`
	}
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	if err := decoder.Decode(&manifest); err != nil {
		return "", err
	}
	manifest.Version = strings.TrimSpace(manifest.Version)
	if manifest.Version == "" {
		return "", errors.New("manifest.json 缺少 version")
	}
	return manifest.Version, nil
}

func fetchAssetText(url string, limit int64) (string, error) {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return "", errors.New("创建扩展版本请求失败")
	}
	setAssetDownloadHeaders(request)
	response, err := updateCheckClient.Do(request)
	if err != nil {
		return "", errors.New("获取远端扩展版本失败")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("扩展版本服务返回 HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return "", errors.New("读取远端扩展版本失败")
	}
	if int64(len(data)) > limit {
		return "", errors.New("远端扩展版本响应过大")
	}
	return strings.TrimSpace(string(data)), nil
}

func downloadAndVerify(url, destination, expectedChecksum string) error {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return errors.New("创建更新下载请求失败")
	}
	setAssetDownloadHeaders(request)
	response, err := updateDownloadClient.Do(request)
	if err != nil {
		return errors.New("连接更新下载服务失败")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("更新下载服务返回 HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxAssetSize {
		return errors.New("更新文件超过大小限制")
	}

	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("创建下载文件: %w", err)
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, maxAssetSize+1))
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("保存下载文件: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("关闭下载文件: %w", closeErr)
	}
	if written > maxAssetSize {
		return errors.New("更新文件超过大小限制")
	}
	actualChecksum := hex.EncodeToString(hash.Sum(nil))
	if actualChecksum != strings.ToLower(expectedChecksum) {
		return errors.New("更新文件的 SHA-256 校验失败")
	}
	return nil
}

func setAssetDownloadHeaders(request *http.Request) {
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "key-crawl-updater")
}

func extractZip(archivePath, destination string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	cleanDestination := filepath.Clean(destination) + string(os.PathSeparator)
	var totalSize uint64
	for _, entry := range reader.File {
		totalSize += entry.UncompressedSize64
		if totalSize > maxAssetSize {
			return fmt.Errorf("扩展压缩包解压后超过大小限制")
		}
		target := filepath.Join(destination, entry.Name)
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanDestination) {
			return fmt.Errorf("压缩包包含不安全路径 %q", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		source, err := entry.Open()
		if err != nil {
			return err
		}
		destinationFile, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, entry.Mode().Perm())
		if err != nil {
			source.Close()
			return err
		}
		_, copyErr := io.Copy(destinationFile, source)
		closeDestinationErr := destinationFile.Close()
		closeSourceErr := source.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeDestinationErr != nil {
			return closeDestinationErr
		}
		if closeSourceErr != nil {
			return closeSourceErr
		}
	}
	return nil
}

func applyUpdate(targetExecutable, extensionDirectory string) error {
	stagedExecutable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取更新助手路径: %w", err)
	}
	stagingDirectory := filepath.Dir(stagedExecutable)
	if err := validateUpdatePaths(stagedExecutable, targetExecutable, extensionDirectory); err != nil {
		return err
	}
	stagedExtension := filepath.Join(stagingDirectory, "extension")
	executableBackup := targetExecutable + ".old"
	extensionBackup := extensionDirectory + ".old"

	if err := retry(15*time.Second, func() error {
		_ = os.Remove(executableBackup)
		return os.Rename(targetExecutable, executableBackup)
	}); err != nil {
		return fmt.Errorf("等待旧程序退出: %w", err)
	}
	rollbackExecutable := true
	defer func() {
		if rollbackExecutable {
			_ = os.Remove(targetExecutable)
			_ = os.Rename(executableBackup, targetExecutable)
		}
	}()

	extensionMoved := false
	if _, statErr := os.Stat(extensionDirectory); statErr == nil {
		_ = os.RemoveAll(extensionBackup)
		if err := os.Rename(extensionDirectory, extensionBackup); err != nil {
			return fmt.Errorf("备份旧扩展: %w", err)
		}
		extensionMoved = true
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("检查旧扩展: %w", statErr)
	}
	rollbackExtension := true
	defer func() {
		if rollbackExtension {
			_ = os.RemoveAll(extensionDirectory)
			if extensionMoved {
				_ = os.Rename(extensionBackup, extensionDirectory)
			}
		}
	}()
	if err := os.Rename(stagedExtension, extensionDirectory); err != nil {
		return fmt.Errorf("安装新扩展: %w", err)
	}
	if err := copyFile(stagedExecutable, targetExecutable); err != nil {
		return fmt.Errorf("安装新程序: %w", err)
	}

	rootDirectory := filepath.Dir(targetExecutable)
	process, err := os.StartProcess(targetExecutable, []string{
		targetExecutable,
		"-cleanup-update", stagingDirectory,
	}, &os.ProcAttr{
		Dir:   rootDirectory,
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	})
	if err != nil {
		return fmt.Errorf("启动新版本: %w", err)
	}
	_ = process.Release()
	rollbackExtension = false
	rollbackExecutable = false
	return nil
}

func validateUpdatePaths(stagedExecutable, targetExecutable, extensionDirectory string) error {
	stagingDirectory := filepath.Dir(stagedExecutable)
	targetDirectory := filepath.Dir(targetExecutable)
	if !strings.EqualFold(filepath.Base(stagedExecutable), executableName) ||
		!strings.EqualFold(filepath.Base(targetExecutable), executableName) ||
		!isUpdateStagingDirectory(targetExecutable, stagingDirectory) ||
		!samePath(extensionDirectory, filepath.Join(targetDirectory, "extension")) {
		return errors.New("更新路径不符合标准安装目录结构")
	}
	return nil
}

func isUpdateStagingDirectory(targetExecutable, stagingDirectory string) bool {
	return strings.HasPrefix(filepath.Base(filepath.Clean(stagingDirectory)), ".key-crawl-update-") &&
		samePath(filepath.Dir(filepath.Clean(stagingDirectory)), filepath.Dir(targetExecutable))
}

func samePath(left, right string) bool {
	leftAbsolute, leftErr := filepath.Abs(left)
	rightAbsolute, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && strings.EqualFold(filepath.Clean(leftAbsolute), filepath.Clean(rightAbsolute))
}

func retry(timeout time.Duration, operation func() error) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := operation(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return lastErr
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
