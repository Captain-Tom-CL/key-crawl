# Key Crawl

本项目包含 Go 本地服务和 Chrome 扩展。

## 目录结构

- `src/cmd/key-crawl/`：服务程序入口
- `src/internal/routes/`：HTTP 路由与业务处理
- `extension/`：Chrome 扩展源码
- `data/`：本地配置及运行数据
- `scripts/`：开发构建与发布脚本
- `bin/`：本地编译产物
- `build/`：客户发布包及远程更新文件
- `requests/`：接口调试请求
- `launch.ps1`：客户启动入口

## 本地构建

运行 `scripts/build.ps1`，输出文件为 `bin/key-crawl.exe`。

## 制作发布包

运行 `scripts/package.ps1`，将生成：

- 自动将 `src/cmd/key-crawl/main.go` 中的整数版本加一
- `build/release/key-crawl-<version>.zip`：完整客户安装包
- `build/artifacts/version.txt`：远端版本文件
- `build/artifacts/key-crawl.exe`：远端程序更新文件
- `build/artifacts/extension.zip`：远端扩展更新文件

`build/release/` 只存放提供给客户的完整 ZIP；`build/artifacts/` 存放需要上传到更新服务器的独立文件。

`launch.ps1` 已被 Git 忽略，不会进入发布产物，需要单独提供给客户。发布前请在其中填写 `$repositoryUrl`。远端目录应提供上述三个成果文件。

`data/settings.json` 包含本地 API Key，已被 Git 忽略。公开发布包使用无密钥的 `data/settings.example.json`，客户通过扩展设置页填写自己的配置。

## GitHub 自动发布

`.github/workflows/release.yml` 会在代码推送到 `main` 或 `master` 时：

1. 使用 Windows Runner 和 PowerShell 7 运行 `scripts/package.ps1`
2. 自动递增并提交 `main.go` 中的版本号
3. 创建 `v<version>` GitHub Release
4. 上传完整客户 ZIP，以及 `version.txt`、`key-crawl.exe`、`extension.zip`

首次启用时，在 GitHub 仓库的 **Settings → Actions → General → Workflow permissions** 中选择 **Read and write permissions**。

公开仓库可在本地 `launch.ps1` 中使用以下更新地址：

```text
https://github.com/<用户名或组织>/<仓库名>/releases/latest/download
```

例如，启动器会访问：

```text
https://github.com/<用户名或组织>/<仓库名>/releases/latest/download/version.txt
```

## 客户目录

客户解压后的目录包含：

- `bin/key-crawl.exe`
- `extension/`
- `data/settings.json`

将单独提供的 `launch.ps1` 放到解压目录根部。客户应通过该脚本启动程序，并在 Chrome 中加载 `extension/` 目录。
