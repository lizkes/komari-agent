package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/komari-monitor/komari-agent/cmd/flags"
	"github.com/komari-monitor/komari-agent/logger"
	monitoring "github.com/komari-monitor/komari-agent/monitoring/unit"
	"github.com/komari-monitor/komari-agent/update"
)

func DoUploadBasicInfoWorks() {
	ticker := time.NewTicker(time.Duration(flags.InfoReportInterval) * time.Minute)
	for range ticker.C {
		err := uploadBasicInfo()
		if err != nil {
			logger.Error("Error uploading basic info: %v", err)
		}
	}
}
func UpdateBasicInfo() {
	err := uploadBasicInfo()
	if err != nil {
		logger.Error("Error uploading basic info: %v", err)
	} else {
		logger.Info("Basic info uploaded successfully")
	}
}
func uploadBasicInfo() error {
	logger.Debug("Starting to collect system information...")

	// 收集系统信息并记录详细内容
	cpu := monitoring.Cpu()
	logger.Debug("CPU Info collected: Name='%s', Cores=%d, Arch='%s'", cpu.CPUName, cpu.CPUCores, cpu.CPUArchitecture)

	osname := monitoring.OSName()
	logger.Debug("OS Name collected: '%s'", osname)

	ipv4, ipv6, ipErr := monitoring.GetIPAddress()
	logger.Debug("IP Address collected: IPv4='%s', IPv6='%s', Error=%v", ipv4, ipv6, ipErr)

	ramInfo := monitoring.Ram()
	logger.Debug("RAM Info collected: Total=%d", ramInfo.Total)

	swapInfo := monitoring.Swap()
	logger.Debug("Swap Info collected: Total=%d", swapInfo.Total)

	diskInfo := monitoring.Disk()
	logger.Debug("Disk Info collected: Total=%d", diskInfo.Total)

	gpuName := monitoring.GpuName()
	logger.Debug("GPU Name collected: '%s'", gpuName)

	virtualization := monitoring.Virtualized()
	logger.Debug("Virtualization collected: '%s'", virtualization)

	data := map[string]interface{}{
		"cpu_name":       cpu.CPUName,
		"cpu_cores":      cpu.CPUCores,
		"arch":           cpu.CPUArchitecture,
		"os":             osname,
		"ipv4":           ipv4,
		"ipv6":           ipv6,
		"mem_total":      ramInfo.Total,
		"swap_total":     swapInfo.Total,
		"disk_total":     diskInfo.Total,
		"gpu_name":       gpuName,
		"virtualization": virtualization,
		"version":        update.CurrentVersion,
	}

	// 记录完整的上报数据
	logger.Debug("Complete data to upload: %+v", data)

	endpoint := strings.TrimSuffix(flags.Endpoint, "/") + "/api/clients/uploadBasicInfo?token=" + flags.Token
	logger.Debug("Upload endpoint: %s", strings.Replace(endpoint, flags.Token, "[TOKEN_HIDDEN]", 1))

	payload, err := json.Marshal(data)
	if err != nil {
		logger.Debug("Failed to marshal JSON payload: %v", err)
		return err
	}
	logger.Debug("JSON payload size: %d bytes", len(payload))

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(string(payload)))
	if err != nil {
		logger.Debug("Failed to create HTTP request: %v", err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	logger.Debug("Sending HTTP request to server...")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Debug("HTTP request failed: %v", err)
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Debug("Failed to read response body: %v", err)
		return err
	}
	message := string(body)

	logger.Debug("Server response: Status=%d, Body='%s'", resp.StatusCode, message)

	if resp.StatusCode != http.StatusOK {
		logger.Debug("Upload failed with status %d: %s", resp.StatusCode, message)
		return fmt.Errorf("status code: %d,%s", resp.StatusCode, message)
	}

	logger.Debug("Basic info upload completed successfully")
	return nil
}
