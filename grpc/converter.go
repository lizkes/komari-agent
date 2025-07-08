package grpc

import (
	"encoding/json"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	monitoring "github.com/komari-monitor/komari-agent/monitoring/unit"
	"github.com/komari-monitor/komari-agent/proto"
	"github.com/komari-monitor/komari-agent/update"
)

// ConvertToMonitorReport 将监控数据转换为protobuf格式
func ConvertToMonitorReport(monitoringData []byte, uuid string) (*proto.MonitorReport, error) {
	// 解析JSON监控数据
	var data map[string]interface{}
	if err := json.Unmarshal(monitoringData, &data); err != nil {
		return nil, err
	}

	report := &proto.MonitorReport{
		Uuid:      uuid,
		Type:      "report",
		UpdatedAt: timestamppb.New(time.Now()),
	}

	// 转换CPU数据
	if cpuData, ok := data["cpu"].(map[string]interface{}); ok {
		cpu := monitoring.Cpu()
		report.Cpu = &proto.CPUReport{
			Name:  cpu.CPUName,
			Cores: int32(cpu.CPUCores),
			Arch:  cpu.CPUArchitecture,
			Usage: getFloat64(cpuData, "usage"),
		}
	}

	// 转换内存数据
	if ramData, ok := data["ram"].(map[string]interface{}); ok {
		report.Ram = &proto.RamReport{
			Total: getInt64(ramData, "total"),
			Used:  getInt64(ramData, "used"),
		}
	}

	// 转换交换分区数据
	if swapData, ok := data["swap"].(map[string]interface{}); ok {
		report.Swap = &proto.RamReport{
			Total: getInt64(swapData, "total"),
			Used:  getInt64(swapData, "used"),
		}
	}

	// 转换负载数据
	if loadData, ok := data["load"].(map[string]interface{}); ok {
		report.Load = &proto.LoadReport{
			Load1:  getFloat64(loadData, "load1"),
			Load5:  getFloat64(loadData, "load5"),
			Load15: getFloat64(loadData, "load15"),
		}
	}

	// 转换磁盘数据
	if diskData, ok := data["disk"].(map[string]interface{}); ok {
		report.Disk = &proto.DiskReport{
			Total: getInt64(diskData, "total"),
			Used:  getInt64(diskData, "used"),
		}
	}

	// 转换网络数据
	if networkData, ok := data["network"].(map[string]interface{}); ok {
		report.Network = &proto.NetworkReport{
			Up:        getInt64(networkData, "up"),
			Down:      getInt64(networkData, "down"),
			TotalUp:   getInt64(networkData, "totalUp"),
			TotalDown: getInt64(networkData, "totalDown"),
		}
	}

	// 转换连接数据
	if connectionsData, ok := data["connections"].(map[string]interface{}); ok {
		report.Connections = &proto.ConnectionsReport{
			Tcp: getInt32(connectionsData, "tcp"),
			Udp: getInt32(connectionsData, "udp"),
		}
	}

	// 转换其他数据
	if uptime, ok := data["uptime"].(float64); ok {
		report.Uptime = int64(uptime)
	}
	if process, ok := data["process"].(float64); ok {
		report.Process = int32(process)
	}
	if message, ok := data["message"].(string); ok {
		report.Message = message
	}

	return report, nil
}

// ConvertToBasicInfoReport 将基础信息转换为监控报告格式
func ConvertToBasicInfoReport(uuid string) *proto.MonitorReport {
	cpu := monitoring.Cpu()
	osname := monitoring.OSName()
	ipv4, ipv6, _ := monitoring.GetIPAddress()

	// 创建基础信息报告，使用特殊的type标识
	report := &proto.MonitorReport{
		Uuid:      uuid,
		Type:      "basic_info",
		UpdatedAt: timestamppb.New(time.Now()),
		Method:    "basic_info",
	}

	// 设置CPU信息
	report.Cpu = &proto.CPUReport{
		Name:  cpu.CPUName,
		Cores: int32(cpu.CPUCores),
		Arch:  cpu.CPUArchitecture,
	}

	// 在message字段中放置基础信息的JSON
	basicInfo := map[string]interface{}{
		"os":             osname,
		"ipv4":           ipv4,
		"ipv6":           ipv6,
		"mem_total":      monitoring.Ram().Total,
		"swap_total":     monitoring.Swap().Total,
		"disk_total":     monitoring.Disk().Total,
		"gpu_name":       monitoring.GpuName(),
		"virtualization": monitoring.Virtualized(),
		"version":        update.CurrentVersion,
	}

	if jsonData, err := json.Marshal(basicInfo); err == nil {
		report.Message = string(jsonData)
	}

	return report
}

// CreatePingResult 创建ping结果
func CreatePingResult(taskID uint32, pingType string, value int32) *proto.PingResult {
	return &proto.PingResult{
		TaskId:     taskID,
		Value:      value,
		PingType:   pingType,
		FinishedAt: timestamppb.New(time.Now()),
		Type:       "ping_result",
	}
}

// 辅助函数
func getFloat64(data map[string]interface{}, key string) float64 {
	if val, ok := data[key].(float64); ok {
		return val
	}
	return 0
}

func getInt64(data map[string]interface{}, key string) int64 {
	if val, ok := data[key].(float64); ok {
		return int64(val)
	}
	return 0
}

func getInt32(data map[string]interface{}, key string) int32 {
	if val, ok := data[key].(float64); ok {
		return int32(val)
	}
	return 0
}
