package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// commandTimeout 定义了 netsh 命令执行的默认超时时间。
const commandTimeout = 10 * time.Second

// globalLogger 是全局的 slog 日志记录器。
var globalLogger *slog.Logger

// --- Custom flag type for slog.Level ---
type logLevelValue struct {
	levelVar *slog.LevelVar
}

// String is part of the flag.Value interface.
func (v *logLevelValue) String() string {
	if v.levelVar == nil {
		return slog.LevelInfo.String() // Default if not set
	}
	return v.levelVar.Level().String()
}

// Set is part of the flag.Value interface.
// It parses the string and sets the slog.Level.
func (v *logLevelValue) Set(s string) error {
	var level slog.Level
	switch strings.ToLower(s) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn", "warning":
		level = slog.LevelWarn
	case "error", "err":
		level = slog.LevelError
	default:
		return fmt.Errorf("invalid log level: %q (must be debug, info, warn, or error)", s)
	}
	if v.levelVar == nil {
		v.levelVar = new(slog.LevelVar) // Ensure it's initialized
	}
	v.levelVar.Set(level)
	return nil
}

// Get returns the current slog.Level.
func (v *logLevelValue) Get() slog.Level {
	if v.levelVar == nil {
		return slog.LevelInfo // Default if somehow not set
	}
	return v.levelVar.Level()
}

// newLogLevelValue creates a new logLevelValue with a default level.
func newLogLevelValue(defaultLevel slog.Level) *logLevelValue {
	lv := new(slog.LevelVar)
	lv.Set(defaultLevel)
	return &logLevelValue{levelVar: lv}
}

// --- End custom flag type ---

// setupSlog initializes the globalLogger with the specified settings.
// 日志可以输出到文件或标准输出。
func setupSlog(logFilePath string, logLevel slog.Level) {
	var output io.Writer = os.Stdout // 默认输出到标准输出

	logHandlerOptions := &slog.HandlerOptions{
		Level: logLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				// 自定义时间格式
				a.Value = slog.StringValue(a.Value.Time().Format("2006-01-02 15:04:05.000"))
			}
			return a
		},
	}

	if logFilePath != "" {
		file, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			// 如果日志文件打开失败，则回退到标准输出并记录错误
			// 在这种早期阶段，globalLogger可能尚未完全初始化，所以直接使用一个新的slog实例打印到stdout
			earlyLogger := slog.New(slog.NewTextHandler(os.Stdout, logHandlerOptions))
			earlyLogger.Error("无法打开或创建日志文件，日志将输出到标准输出", slog.String("路径", logFilePath), slog.Any("错误", err))
			// 保持 output 为 os.Stdout
		} else {
			// 如果需要同时输出到文件和控制台，可以使用 io.MultiWriter
			// output = io.MultiWriter(os.Stdout, file)
			output = file
			fmt.Printf("日志将写入到: %s\n", logFilePath) // 初始时仍在控制台打印一条提示
		}
	} else {
		fmt.Println("日志将输出到标准输出。")
	}

	globalLogger = slog.New(slog.NewTextHandler(output, logHandlerOptions))
	slog.SetDefault(globalLogger) // 设置为默认记录器，方便全局使用 slog.Info 等
	globalLogger.Info("日志记录器初始化完成")
}

// runNetshCommand 执行 netsh 命令并返回 stdout, stderr 和错误。
// 它包含超时和隐藏窗口的逻辑。
func runNetshCommand(timeout time.Duration, args ...string) (stdout string, stderr string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "netsh", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} // 隐藏命令执行时弹出的控制台窗口

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()

	if ctx.Err() == context.DeadlineExceeded {
		return stdout, stderr, fmt.Errorf("命令 '%s' 执行超时 (%v)", strings.Join(args, " "), timeout)
	}
	if err != nil {
		return stdout, stderr, fmt.Errorf("命令 '%s' 执行失败: %w, stderr: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr))
	}
	return stdout, stderr, nil
}

// getWlanInterface尝试自动检测系统上的第一个无线网络接口名称。
// 这对于用户未明确指定接口名称时非常有用。
func getWlanInterface() (string, error) {
	slog.Debug("开始自动检测无线网络接口...")
	stdout, _, err := runNetshCommand(commandTimeout, "wlan", "show", "interfaces")
	if err != nil {
		return "", fmt.Errorf("执行 'netsh wlan show interfaces' 失败 (getWlanInterface): %w", err)
	}

	lines := strings.SplitSeq(stdout, "\n")
	for line := range lines {
		trimmedLine := strings.TrimSpace(line)
		// "名称" 来自用户提供的成功运行的程序中的关键字
		if strings.HasPrefix(trimmedLine, "名称") || strings.HasPrefix(trimmedLine, "Name") {
			parts := strings.SplitN(trimmedLine, ":", 2)
			if len(parts) == 2 {
				ifaceName := strings.TrimSpace(parts[1])
				if ifaceName != "" {
					slog.Info("自动检测到无线网络接口", slog.String("接口名称", ifaceName))
					return ifaceName, nil
				}
			}
		}
	}
	return "", errors.New("未能通过 'netsh wlan show interfaces' 自动检测到无线网络接口 (getWlanInterface)")
}

// isConnected 检查指定的无线接口是否已连接到目标SSID。
// 它解析 'netsh wlan show interfaces' 的输出。
func isConnected(targetSSID string, interfaceName string) (bool, error) {
	slog.Debug("检查连接状态...", slog.String("目标SSID", targetSSID), slog.String("接口", interfaceName))
	stdout, stderr, err := runNetshCommand(commandTimeout, "wlan", "show", "interfaces")
	if err != nil {
		// 特殊处理WLAN服务未运行或适配器问题
		errMsg := stderr
		if errMsg == "" { // exec.ExitError 可能没有 stderr 输出
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				errMsg = fmt.Sprintf("netsh 命令返回退出状态 %d (可能是WLAN AutoConfig服务未运行, 或Wi-Fi适配器被禁用/不存在)", exitErr.ExitCode())
			}
		}
		return false, fmt.Errorf("执行 'netsh wlan show interfaces' 失败 (isConnected): %w, stderr: %s", err, errMsg)
	}

	lines := strings.Split(stdout, "\n")
	inTargetInterfaceBlock := false
	var blockParsedSSID string
	var blockParsedState string // e.g., "已连接", "断开连接"

	// 关键字来自用户提供的成功运行的程序
	const keywordInterfaceName = "名称"   // "名称"
	const keywordSSID = "SSID"          // "SSID"
	const keywordState = "状态"           // "状态"
	const keywordConnectedState = "已连接" // "已连接"

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		if strings.HasPrefix(trimmedLine, keywordInterfaceName) && strings.Contains(trimmedLine, ":") {
			parts := strings.SplitN(trimmedLine, ":", 2)
			if len(parts) == 2 {
				currentInterfaceNameInOutput := strings.TrimSpace(parts[1])
				if currentInterfaceNameInOutput == interfaceName {
					inTargetInterfaceBlock = true
					blockParsedSSID = "" // 重置当前块的解析状态
					blockParsedState = ""
				} else {
					// 如果我们正在解析一个接口块，并且遇到了一个新的接口块声明，
					// 而我们之前解析的块不是目标接口块，则将 inTargetInterfaceBlock 设置为 false。
					// 如果已经是目标接口块，则不应改变，继续解析。
					if inTargetInterfaceBlock {
						// 已处理完目标接口块，可以提前判断
						// (或者如果希望只处理第一个匹配的接口块，这里可以break或返回)
					}
					inTargetInterfaceBlock = false
				}
			}
			continue // 继续下一行，避免在同一行处理接口名称和其他属性
		}

		if inTargetInterfaceBlock {
			// SSID 行不应包含 "AP BSSID" 来避免混淆
			if strings.HasPrefix(trimmedLine, keywordSSID) && !strings.Contains(trimmedLine, "AP BSSID") && strings.Contains(trimmedLine, ":") {
				parts := strings.SplitN(trimmedLine, ":", 2)
				if len(parts) == 2 {
					blockParsedSSID = strings.TrimSpace(parts[1])
				}
			} else if strings.HasPrefix(trimmedLine, keywordState) && strings.Contains(trimmedLine, ":") {
				parts := strings.SplitN(trimmedLine, ":", 2)
				if len(parts) == 2 {
					blockParsedState = strings.TrimSpace(parts[1])
				}
			}

			// 当SSID和状态都解析出来后，进行判断
			if blockParsedSSID != "" && blockParsedState != "" {
				if blockParsedSSID == targetSSID && blockParsedState == keywordConnectedState {
					slog.Debug("目标SSID已连接", slog.String("SSID", blockParsedSSID), slog.String("状态", blockParsedState))
					return true, nil
				}
				// 如果SSID匹配但状态不是"已连接"，则说明未连接到目标 (或者连接了但状态不对)
				if blockParsedSSID == targetSSID && blockParsedState != keywordConnectedState {
					slog.Debug("目标SSID存在但未连接或状态异常", slog.String("SSID", blockParsedSSID), slog.String("状态", blockParsedState))
					return false, nil // 明确未连接到目标
				}
			}
		}
	}
	slog.Debug("遍历完接口信息，未确认连接到目标SSID")
	return false, nil // 遍历完成，未找到匹配且连接的SSID
}

// isNetworkAvailable 检查目标SSID是否存在于指定接口的可见网络列表中。
func isNetworkAvailable(targetSSID string, interfaceName string) (bool, error) {
	slog.Debug("检查网络是否可见...", slog.String("目标SSID", targetSSID), slog.String("接口", interfaceName))
	stdout, stderr, err := runNetshCommand(commandTimeout, "wlan", "show", "networks", fmt.Sprintf("interface=%q", interfaceName), "mode=bssid")
	if err != nil {
		if strings.Contains(stderr, "没有无线网络可见") || strings.Contains(stderr, "No wireless networks are currently visible") {
			slog.Info("指定接口上没有可见的无线网络", slog.String("接口", interfaceName))
			return false, nil
		}
		return false, fmt.Errorf("执行 'netsh wlan show networks' 失败 (isNetworkAvailable): %w", err)
	}

	lines := strings.Split(stdout, "\n")
	const keywordSSID = "SSID "

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, keywordSSID) {
			parts := strings.SplitN(trimmedLine, ":", 2)
			if len(parts) == 2 {
				scannedSSID := strings.TrimSpace(parts[1])
				slog.Debug("扫描到可见SSID", slog.String("可见SSID", scannedSSID))
				if scannedSSID == targetSSID {
					slog.Info("目标SSID可见", slog.String("SSID", targetSSID), slog.String("接口", interfaceName))
					return true, nil
				}
			}
		}
	}

	slog.Info("目标SSID在扫描的网络中不可见", slog.String("SSID", targetSSID), slog.String("接口", interfaceName))
	return false, nil
}

// connectToWifi 尝试将指定的无线接口连接到目标SSID。
func connectToWifi(targetSSID string, interfaceName string) error {
	slog.Info("尝试连接到WiFi...", slog.String("SSID", targetSSID), slog.String("接口", interfaceName))
	_, stderr, err := runNetshCommand(commandTimeout*2, // 连接操作可能需要更长时间
		"wlan", "connect", fmt.Sprintf("name=%q", targetSSID), fmt.Sprintf("interface=%q", interfaceName))

	if err != nil {
		slog.Error("连接命令发送失败", slog.String("SSID", targetSSID), slog.Any("错误", err), slog.String("stderr", stderr))
		return fmt.Errorf("netsh wlan connect 命令失败: %w", err)
	}
	slog.Info("连接命令已成功发送", slog.String("SSID", targetSSID))
	return nil
}

func main() {
	// --- 定义命令行参数 ---
	targetSSIDFlag := flag.String("ssid", "", "要连接的WiFi SSID (必需)")
	wifiInterfaceFlag := flag.String("interface", "", "无线网络接口名称 (例如 WLAN, Wi-Fi)。如果为空，则尝试自动检测。")
	checkIntervalFlag := flag.Duration("interval", 15*time.Second, "检查WiFi连接状态的时间间隔 (例如: 10s, 1m)")
	logFilePathFlag := flag.String("logfile", "", "日志文件路径。如果为空，则输出到标准输出。")

	// 使用自定义的 logLevelValue 类型
	logLevelFlag := newLogLevelValue(slog.LevelInfo)                      // 默认设置为 Info
	flag.Var(logLevelFlag, "loglevel", "日志级别 (debug, info, warn, error)") // 正确的行

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: %s -ssid <目标SSID> [选项]\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "选项:")
		flag.PrintDefaults()
	}
	flag.Parse()

	// --- 初始化日志 ---
	setupSlog(*logFilePathFlag, logLevelFlag.Get()) // 使用 Get() 获取 slog.Level

	// --- 参数校验 ---
	if *targetSSIDFlag == "" {
		slog.Error("错误: 必须通过 -ssid 参数指定目标WiFi SSID")
		flag.Usage()
		os.Exit(1)
	}

	slog.Info("WiFi自动重连程序启动",
		slog.String("目标SSID", *targetSSIDFlag),
		slog.String("指定接口", *wifiInterfaceFlag),
		slog.Duration("检查间隔", *checkIntervalFlag),
		slog.String("日志级别", logLevelFlag.Get().String()),
	)

	effectiveIfaceName := *wifiInterfaceFlag
	if effectiveIfaceName == "" {
		slog.Info("未指定网络接口名称，尝试自动检测...")
		var err error
		effectiveIfaceName, err = getWlanInterface()
		if err != nil {
			slog.Error("无法自动检测无线网络接口，请使用 -interface 参数指定。", slog.Any("错误", err))
			os.Exit(1)
		}
	}
	slog.Info("将使用网络接口", slog.String("接口名称", effectiveIfaceName))

	// --- 主循环 ---
	ticker := time.NewTicker(*checkIntervalFlag)
	defer ticker.Stop()

	performCheckAndConnect(*targetSSIDFlag, effectiveIfaceName)

	for {
		select {
		case currentTime := <-ticker.C:
			slog.Debug("定时检查触发", slog.Time("时间", currentTime))
			performCheckAndConnect(*targetSSIDFlag, effectiveIfaceName)
		}
	}
}

// performCheckAndConnect 执行一次完整的检查和连接尝试逻辑。
func performCheckAndConnect(targetSSID, interfaceName string) {
	slog.Info("开始检查WiFi连接状态...", slog.String("SSID", targetSSID), slog.String("接口", interfaceName))
	connected, err := isConnected(targetSSID, interfaceName)
	if err != nil {
		slog.Error("检查连接状态时出错", slog.Any("错误", err))
		return
	}

	if connected {
		slog.Info("已连接到目标WiFi", slog.String("SSID", targetSSID))
	} else {
		slog.Warn("未连接到目标WiFi，开始处理...", slog.String("目标SSID", targetSSID))
		networkVisible, availErr := isNetworkAvailable(targetSSID, interfaceName)
		if availErr != nil {
			slog.Error("检查网络可见性时出错", slog.String("SSID", targetSSID), slog.Any("错误", availErr))
			return
		}

		if networkVisible {
			slog.Info("目标网络可见，尝试连接...", slog.String("SSID", targetSSID))
			connectErr := connectToWifi(targetSSID, interfaceName)
			if connectErr != nil {
				slog.Error("连接尝试失败", slog.String("SSID", targetSSID), slog.Any("错误", connectErr))
			} else {
				slog.Info("连接命令已发送，将在下一个周期确认连接状态。")
			}
		} else {
			slog.Warn("目标网络当前不可见，跳过连接尝试。", slog.String("SSID", targetSSID))
		}
	}
}
