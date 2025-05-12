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
	targetSSID        = "qaz_5G"
	wifiInterfaceName = "WLAN" // 例如 "WLAN" 或 "Wi-Fi"
	checkInterval     = 15 * time.Second
	logFilePath       = "D:\\Go编程\\go-reconnectwifi\\GoWiFiReconnectLog.txt"
)

// --- 配置区域结束 ---

var logger *log.Logger

func setupLogger() {
	logFlags := log.Ldate | log.Ltime
	logPrefix := "WIFI_RECONNECT: "
	if logFilePath == "" {
		logger = log.New(os.Stdout, logPrefix, logFlags)
		logger.Println("日志将输出到标准输出。")
		return
	}
	file, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		logger = log.New(os.Stdout, "ERROR_LOGGER: ", logFlags)
		logger.Printf("无法打开或创建日志文件 '%s': %v。日志将输出到标准输出。", logFilePath, err)
		return
	}
	logger = log.New(file, logPrefix, logFlags)
	logger.Println("日志记录器初始化完成，日志将写入到:", logFilePath)
}

func getWlanInterface() (string, error) {
	if logger == nil {
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
				if detectedInterfaceName != "" {
					break
				}
			}
		}
	}
	if detectedInterfaceName == "" {
		return "", fmt.Errorf("未能通过 'netsh wlan show interfaces' 自动检测到无线网络接口 (getWlanInterface)")
	}
	return detectedInterfaceName, nil
}

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
	if err != nil {
		errMsg := stderrCmd.String()
		if errMsg == "" && err.Error() == "exit status 1" {
			errMsg = "netsh wlan show interfaces returned exit status 1 (可能是WLAN AutoConfig服务未运行, 或Wi-Fi适配器被禁用/不存在)"
		}
		return false, fmt.Errorf("执行 'netsh wlan show interfaces' 失败 (isConnected): %v, stderr: %s", err, errMsg)
	}
	output := out.String()
	lines := strings.Split(output, "\n")
	inTargetInterfaceBlock := false
	var blockParsedSSID string
	var blockParsedState string
	for _, line := range lines {
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
			if blockParsedSSID == targetSSID_param && blockParsedState != "" && blockParsedState != "已连接" {
				return false, nil
			}
		}
	}
	return false, nil
}

// isNetworkAvailable 检查目标SSID是否在可见网络列表中
func isNetworkAvailable(targetSSID_param string, ifaceName_param string) (bool, error) {
	if logger == nil {
		fmt.Println("CRITICAL_ERROR: isNetworkAvailable 调用时 logger 未初始化!")
		return false, fmt.Errorf("logger not initialized in isNetworkAvailable")
	}

	// 指定接口进行扫描，确保是针对我们关心的适配器
	cmd := exec.Command("netsh", "wlan", "show", "networks", fmt.Sprintf("interface=\"%s\"", ifaceName_param), "mode=bssid")
	var out bytes.Buffer
	var stderrCmd bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderrCmd

	err := cmd.Run()
	/*
		// 调试时可以取消注释以查看原始输出
		if logger != nil {
			logger.Printf("DEBUG isNetworkAvailable: 'netsh wlan show networks interface=\"%s\" mode=bssid' RAW OUTPUT ====\n%s\n==== END RAW OUTPUT. Stderr: '%s'", ifaceName_param, out.String(), stderrCmd.String())
		}
	*/
	if err != nil {
		errMsg := stderrCmd.String()
		if strings.Contains(errMsg, "没有无线网络可见") || strings.Contains(errMsg, "No wireless networks are currently visible") {
			logger.Printf("INFO: isNetworkAvailable: No networks visible on interface '%s'.", ifaceName_param)
			return false, nil
		}
		return false, fmt.Errorf("执行 'netsh wlan show networks' 失败 (isNetworkAvailable): %v, stderr: %s", err, errMsg)
	}

	output := out.String()
	lines := strings.SplitSeq(output, "\n")

	// 确认输出是针对我们期望的接口 (可选加强)
	// 当前逻辑依赖于命令中指定 interface 参数，并直接解析SSID列表
	// 如果需要更严格的接口确认，可以取消注释并调整这里的逻辑
	/*
		interfaceHeaderFound := false
		for _, line := range lines {
			trimmedLine := strings.TrimSpace(line)
			if strings.HasPrefix(trimmedLine, "接口名称") || strings.HasPrefix(trimmedLine, "Interface name") {
				parts := strings.SplitN(trimmedLine, ":", 2)
				if len(parts) == 2 {
					parsedIfaceName := strings.TrimSpace(parts[1])
					if parsedIfaceName == ifaceName_param {
						interfaceHeaderFound = true
						break
					} else {
						logger.Printf("WARNING: isNetworkAvailable: 'show networks' output is for interface '%s', but expected '%s'.", parsedIfaceName, ifaceName_param)
						return false, fmt.Errorf("show networks output for unexpected interface: %s", parsedIfaceName)
					}
				}
			}
		}
		if !interfaceHeaderFound && !strings.Contains(output, "当前有") && !strings.Contains(output, "networks currently visible"){
			logger.Printf("WARNING: isNetworkAvailable: Could not confirm interface name in 'show networks' output or no network visibility line found. Output: \n%s", output)
			return false, nil
		}
	*/

	for line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "SSID ") {
			parts := strings.SplitN(trimmedLine, ":", 2)
			if len(parts) == 2 {
				scannedSSID := strings.TrimSpace(parts[1])
				// logger.Printf("DEBUG isNetworkAvailable: Found visible SSID: '%s'", scannedSSID) // 调试日志
				if scannedSSID == targetSSID_param {
					logger.Printf("INFO: isNetworkAvailable: Target SSID '%s' is visible on interface '%s'.", targetSSID_param, ifaceName_param)
					return true, nil
				}
			}
		}
	}

	logger.Printf("INFO: isNetworkAvailable: Target SSID '%s' is NOT visible on interface '%s' after checking all scanned networks.", targetSSID_param, ifaceName_param)
	return false, nil
}

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
	setupLogger()
	logger.Println("INFO: Go Wi-Fi Auto Reconnect Program Started.")
	logger.Printf("INFO: Target SSID: '%s'", targetSSID)
	logger.Printf("INFO: Specified Wi-Fi Interface: '%s'", wifiInterfaceName)
	logger.Printf("INFO: Check Interval: %v", checkInterval)

	effectiveIfaceName := wifiInterfaceName
	if effectiveIfaceName == "" {
		logger.Println("INFO: Wi-Fi interface name not specified in config, attempting auto-detection...")
		var err error
		effectiveIfaceName, err = getWlanInterface()
		if err != nil {
			logger.Printf("FATAL: Could not auto-determine Wi-Fi interface name: %v. Please specify 'wifiInterfaceName' in config.", err)
			logger.Println("INFO: Program Exiting.")
			if logFilePath == "" {
				fmt.Println("Press Enter to exit...")
				fmt.Scanln()
			}
			return
		}
		logger.Printf("INFO: Auto-detected and will use Wi-Fi interface: '%s'", effectiveIfaceName)
	} else {
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
				logger.Printf("ERROR: Error checking connection status (isConnected): %v", err)
				continue
			}

			if !connected {
				logger.Printf("INFO: Not connected to target SSID '%s'. Checking if network is visible...", targetSSID)
				networkVisible, availErr := isNetworkAvailable(targetSSID, effectiveIfaceName)
				if availErr != nil {
					logger.Printf("ERROR: Error checking network availability for SSID '%s' (isNetworkAvailable): %v", targetSSID, availErr)
				} else if networkVisible {
					logger.Printf("INFO: Target SSID '%s' is visible. Attempting reconnect...", targetSSID)
					connectErr := connectToWifi(targetSSID, effectiveIfaceName)
					if connectErr != nil {
						logger.Printf("ERROR: Failed to connect to '%s' (connectToWifi): %v", targetSSID, connectErr)
					}
				} else {
					logger.Printf("INFO: Target SSID '%s' is not visible. Skipping connection attempt for this cycle.", targetSSID)
				}
			} else {
				logger.Printf("INFO: Already connected to target SSID '%s'. Status normal.", targetSSID)
			}
		}
	}
}
