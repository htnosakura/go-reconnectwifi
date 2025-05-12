package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
	// "io" // 如果需要同时输出日志到控制台和文件，取消此行注释
)

// --- 配置区域 ---
// !!! 请务必根据您的实际网络情况修改以下配置 !!!
const (
	// targetSSID: 您希望自动连接的Wi-Fi网络名称 (SSID)
	targetSSID = "qaz_5G"

	// wifiInterfaceName: 您的无线网络适配器在系统中显示的名称。
	// 例如 "WLAN" 或 "Wi-Fi"
	wifiInterfaceName = "WLAN"

	// checkInterval: 检查Wi-Fi连接状态的时间间隔
	checkInterval = 30 * time.Second

	// logFilePath: 日志文件的完整路径。如果留空，日志将输出到控制台。
	logFilePath = "D:\\Go编程\\go-reconnectwifi\\GoWiFiReconnectLog.txt"
)

// --- 配置区域结束 ---

var logger *log.Logger

func setupLogger() {
	// 对于最终版本，可以移除 log.Lshortfile 以减少日志冗余
	logFlags := log.Ldate | log.Ltime

	if logFilePath == "" {
		logger = log.New(os.Stdout, "WIFI_RECONNECT: ", logFlags)
		logger.Println("日志将输出到标准输出。")
		return
	}

	file, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		// 如果日志文件创建失败，回退到标准输出，并添加明确的错误前缀
		logger = log.New(os.Stdout, "ERROR_LOGGER: ", logFlags)
		logger.Printf("无法打开或创建日志文件 '%s': %v。日志将输出到标准输出。", logFilePath, err)
		return
	}
	// 同时输出到文件和控制台 (如果需要，取消下一行和上面 import "io" 的注释)
	// mw := io.MultiWriter(os.Stdout, file)
	// logger = log.New(mw, "WIFI_RECONNECT: ", logFlags)

	// 只输出到文件
	logger = log.New(file, "WIFI_RECONNECT: ", logFlags)
	logger.Println("日志记录器初始化完成，日志将写入到:", logFilePath)
}

// getWlanInterface 尝试自动获取活动的无线网络接口名称
func getWlanInterface() (string, error) {
	if logger == nil {
		// 这种情况不应该发生，因为 main 会先调用 setupLogger
		fmt.Println("CRITICAL_ERROR: getWlanInterface 调用时 logger 未初始化!")
		return "", fmt.Errorf("logger not initialized in getWlanInterface")
	}

	cmd := exec.Command("netsh", "wlan", "show", "interfaces")
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("执行 'netsh wlan show interfaces' 失败 (getWlanInterface): %v, stderr: %s", err, stderr.String())
	}

	output := out.String()
	lines := strings.Split(output, "\n")
	var detectedInterfaceName string

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "名称") || strings.HasPrefix(trimmedLine, "Name") {
			parts := strings.SplitN(trimmedLine, ":", 2)
			if len(parts) == 2 {
				detectedInterfaceName = strings.TrimSpace(parts[1])
				// 保留一个非DEBUG级别的日志，用于确认自动检测结果（如果启用了自动检测）
				// logger.Printf("INFO getWlanInterface: 自动检测到可能的接口名称: '%s'", detectedInterfaceName)
				if detectedInterfaceName != "" {
					break // 假设第一个找到的就是主WLAN接口
				}
			}
		}
	}

	if detectedInterfaceName == "" {
		return "", fmt.Errorf("未能通过 'netsh wlan show interfaces' 自动检测到无线网络接口。请在配置中指定 wifiInterfaceName (getWlanInterface)")
	}
	// 主函数中会记录最终使用的接口名，这里不再重复记录
	return detectedInterfaceName, nil
}

// isConnected 检查是否连接到目标SSID (修正顺序问题)
func isConnected(targetSSID_param string, ifaceName_param string) (bool, error) {
	if logger == nil {
		fmt.Println("CRITICAL_ERROR: isConnected 调用时 logger 未初始化!")
		return false, fmt.Errorf("logger not initialized in isConnected")
	}

	cmd := exec.Command("netsh", "wlan", "show", "interfaces")
	var out bytes.Buffer
	var stderrCmd bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderrCmd

	err := cmd.Run()

	// 必要时可以取消注释以下行以进行精细调试，但正常运行时建议注释掉
	/*
		if logger != nil {
			logger.Printf("DEBUG isConnected: 'netsh wlan show interfaces' START RAW OUTPUT for interface '%s' ====\n%s\n==== END RAW OUTPUT. Stderr: '%s'", ifaceName_param, out.String(), stderrCmd.String())
		}
	*/

	if err != nil {
		errMsg := stderrCmd.String()
		if errMsg == "" && err.Error() == "exit status 1" {
			errMsg = "netsh wlan show interfaces returned exit status 1 (可能是WLAN AutoConfig服务未运行, 或Wi-Fi适配器被禁用/不存在)"
		}
		// 这个错误是执行netsh命令本身的错误，是重要的，需要记录
		return false, fmt.Errorf("执行 'netsh wlan show interfaces' 失败 (isConnected): %v, stderr: %s", err, errMsg)
	}

	output := out.String()
	lines := strings.Split(output, "\n")

	inTargetInterfaceBlock := false
	var blockParsedSSID string
	var blockParsedState string

	for _, line := range lines { // 移除行号 'i' 的使用，除非进行深度调试
		trimmedLine := strings.TrimSpace(line)

		if strings.HasPrefix(trimmedLine, "名称") {
			parts := strings.SplitN(trimmedLine, ":", 2)
			if len(parts) == 2 {
				currentInterfaceNameInOutput := strings.TrimSpace(parts[1])
				if currentInterfaceNameInOutput == ifaceName_param {
					inTargetInterfaceBlock = true
					blockParsedSSID = ""
					blockParsedState = ""
				} else {
					inTargetInterfaceBlock = false
				}
			}
			continue
		}

		if inTargetInterfaceBlock {
			if strings.HasPrefix(trimmedLine, "SSID") && !strings.Contains(trimmedLine, "AP BSSID") {
				parts := strings.SplitN(trimmedLine, ":", 2)
				if len(parts) == 2 {
					blockParsedSSID = strings.TrimSpace(parts[1])
				}
			}

			if strings.HasPrefix(trimmedLine, "状态") {
				parts := strings.SplitN(trimmedLine, ":", 2)
				if len(parts) == 2 {
					blockParsedState = strings.TrimSpace(parts[1])
				}
			}

			if blockParsedSSID == targetSSID_param && blockParsedState == "已连接" {
				return true, nil
			}
			// 如果目标SSID已找到，但状态已知且非“已连接”，则认为未连接
			if blockParsedSSID == targetSSID_param && blockParsedState != "" && blockParsedState != "已连接" {
				return false, nil // 明确未连接到目标
			}
		}
	}
	// 遍历完成，未找到匹配的已连接状态
	return false, nil
}

// connectToWifi 尝试连接到指定的SSID
func connectToWifi(ssid string, iface string) error {
	if logger == nil {
		fmt.Println("CRITICAL_ERROR: connectToWifi 调用时 logger 未初始化!")
		return fmt.Errorf("logger not initialized in connectToWifi")
	}

	logger.Printf("INFO: Attempting to connect to Wi-Fi: '%s' (Interface: '%s')", ssid, iface)

	cmd := exec.Command("netsh", "wlan", "connect", fmt.Sprintf("name=\"%s\"", ssid), fmt.Sprintf("interface=\"%s\"", iface))
	var out bytes.Buffer
	var stderrCmd bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderrCmd

	err := cmd.Run()
	stdoutStr := out.String()
	stderrStr := stderrCmd.String()

	if err != nil {
		logger.Printf("ERROR: Connection command failed. Error: %v, stdout: '%s', stderr: '%s'", err, stdoutStr, stderrStr)
		return fmt.Errorf("netsh wlan connect failed: %v, stderr: %s", err, stderrStr)
	}

	logger.Printf("INFO: Connection command sent for '%s'. Stdout: '%s', Stderr: '%s'", ssid, stdoutStr, stderrStr)
	return nil
}

func main() {
	setupLogger() // 初始化logger

	logger.Println("INFO: Go Wi-Fi Auto Reconnect Program Started.")
	logger.Printf("INFO: Target SSID: '%s'", targetSSID)
	logger.Printf("INFO: Specified Wi-Fi Interface: '%s'", wifiInterfaceName) // 如果为空，后续会更新
	logger.Printf("INFO: Check Interval: %v", checkInterval)

	effectiveIfaceName := wifiInterfaceName
	if effectiveIfaceName == "" {
		logger.Println("INFO: Wi-Fi interface name not specified in config, attempting auto-detection...")
		var err error
		effectiveIfaceName, err = getWlanInterface()
		if err != nil {
			logger.Printf("FATAL: Could not auto-determine Wi-Fi interface name: %v. Please specify 'wifiInterfaceName' in the program configuration.", err)
			logger.Println("INFO: Program Exiting.")
			if logFilePath == "" { // 如果日志只输出到控制台，暂停一下让用户看到错误
				fmt.Println("Press Enter to exit...")
				fmt.Scanln()
			}
			return
		}
		logger.Printf("INFO: Auto-detected and will use Wi-Fi interface: '%s'", effectiveIfaceName)
	} else {
		// 如果在const中已指定，则此日志确认使用指定值
		logger.Printf("INFO: Will use specified Wi-Fi interface: '%s'", effectiveIfaceName)
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case currentTime := <-ticker.C:
			logger.Printf("INFO: Tick at %s. Checking Wi-Fi status...", currentTime.Format("2006-01-02 15:04:05"))

			connected, err := isConnected(targetSSID, effectiveIfaceName)

			if err != nil {
				logger.Printf("ERROR: Error checking connection status: %v", err)
				continue // 继续下一次尝试，而不是退出
			}

			if !connected {
				logger.Printf("INFO: Not connected to target SSID '%s'. Attempting reconnect...", targetSSID)
				connectErr := connectToWifi(targetSSID, effectiveIfaceName)
				if connectErr != nil {
					logger.Printf("ERROR: Failed to connect to '%s': %v", targetSSID, connectErr)
				} else {
					// 通常 netsh connect 的成功输出在 connectToWifi 函数中已记录
					// logger.Printf("INFO: Connection command for '%s' processed. Will re-check status on next tick.", targetSSID)
				}
			} else {
				logger.Printf("INFO: Already connected to target SSID '%s'. Status normal.", targetSSID)
			}
		}
	}
}
