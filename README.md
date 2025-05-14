# Windows WiFi Auto-Reconnect Utility (Windows WiFi 自动重连工具)

[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)

一个 Windows 命令行工具，用于自动重新连接到指定的 WiFi 网络。可指定WiFi名（暂只支持一个WiFi）、重连时长、适配器、日志。

A Windows command line tool for automatically reconnecting to a specified WiFi network. You can specify the WiFi name (currently only supports one WiFi), reconnection time, adapter, and log.

---

## 中文 (Chinese)

### 先决条件

  * Windows 操作系统。
  * Go 1.21 或更高版本 (如果您需要从源码编译)。

### 从源码编译

1.  克隆仓库 (或下载源代码):
    ```bash
    git clone [https://github.com/YourGitHubUsername/YourRepoName.git](https://github.com/YourGitHubUsername/YourRepoName.git)
    cd YourRepoName
    ```
2.  编译可执行文件 (请确保您的主 Go 文件是 `wifi_reconnect_slog.go` 或相应更新命令):
    ```bash
    go build -o wifi_reconnector.exe wifi_reconnect_slog.go
    ```

### 使用方法 (命令行)

通过命令提示符 (CMD) 或 PowerShell 使用以下参数运行程序：

  * `-ssid <字符串>`: **(必需)** 要连接的 WiFi 网络的 SSID (名称)。如果 SSID 包含空格，请用引号括起来 (例如：`"我的 WiFi"`)。
  * `-interface <字符串>`: (可选) 您的无线网络接口的名称 (例如："WLAN", "Wi-Fi")。如果省略，程序将尝试自动检测。
  * `-interval <时长>`: (可选) 状态检查的时间间隔 (例如：`10s`, `1m`, `30s`)。默认：`15s`。
  * `-logfile <字符串>`: (可选) 日志文件的路径。如果省略，日志将输出到控制台。例如：`wifi.log` 或 `C:\logs\wifi.log`。
  * `-loglevel <字符串>`: (可选) 日志级别：`debug`, `info`, `warn`, `error`。默认：`info`。

**示例:**

```bash
.\wifi_reconnector.exe -ssid "你的WiFi名称" -interface "WLAN" -interval 30s -logfile "reconnect.log" -loglevel debug
```

### 许可证

本程序采用 GNU通用公共许可证v3.0 授权。更多详情请参阅 [LICENSE](https://www.google.com/search?q=LICENSE) 文件。

-----

## English

### Prerequisites

* Windows Operating System.
* Go 1.21 or newer (for building from source).

### Building from Source

1.  Clone the repository (or download the source code):
    ```bash
    git clone [https://github.com/YourGitHubUsername/YourRepoName.git](https://github.com/YourGitHubUsername/YourRepoName.git)
    cd YourRepoName
    ```
2.  Build the executable (ensure your main Go file is `wifi_reconnect_slog.go` or update the command):
    ```bash
    go build -o wifi_reconnector.exe wifi_reconnect_slog.go
    ```

### Usage (Command-Line)

Run the program from a Command Prompt (CMD) or PowerShell using the following parameters:

* `-ssid <string>`: **(Required)** The SSID (name) of the WiFi network. Enclose in quotes if it contains spaces (e.g., `"My WiFi"`).
* `-interface <string>`: (Optional) Name of your wireless network interface (e.g., "WLAN", "Wi-Fi"). If omitted, attempts auto-detection.
* `-interval <duration>`: (Optional) Time interval for status checks (e.g., `10s`, `1m`, `30s`). Default: `15s`.
* `-logfile <string>`: (Optional) Path to a log file. If omitted, logs go to the console. Example: `wifi.log` or `C:\logs\wifi.log`.
* `-loglevel <string>`: (Optional) Logging level: `debug`, `info`, `warn`, `error`. Default: `info`.

**Example:**

```bash
.\wifi_reconnector.exe -ssid "YourWiFiSSID" -interface "WLAN" -interval 30s -logfile "reconnect.log" -loglevel debug
````

### License

This program is licensed under the GNU General Public License v3.0. See the [LICENSE](https://www.google.com/search?q=LICENSE) file for more details.


