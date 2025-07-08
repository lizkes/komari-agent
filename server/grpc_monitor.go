package server

import (
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/komari-monitor/komari-agent/cmd/flags"
	grpcClient "github.com/komari-monitor/komari-agent/grpc"
	"github.com/komari-monitor/komari-agent/monitoring"
)

// GRPCMonitorManager gRPC监控管理器
type GRPCMonitorManager struct {
	client *grpcClient.GRPCClient
	uuid   string
}

// NewGRPCMonitorManager 创建新的gRPC监控管理器
func NewGRPCMonitorManager() *GRPCMonitorManager {
	return &GRPCMonitorManager{
		client: grpcClient.NewGRPCClient(),
		uuid:   uuid.New().String(),
	}
}

// Start 启动gRPC监控服务
func (m *GRPCMonitorManager) Start() error {
	// 连接到gRPC服务器
	if err := m.client.Connect(); err != nil {
		return err
	}

	// 启动监控流
	if err := m.client.StartMonitorStream(); err != nil {
		return err
	}

	log.Println("gRPC监控服务已启动")

	// 首次发送基础信息
	if err := m.sendBasicInfo(); err != nil {
		log.Printf("发送基础信息失败: %v", err)
	}

	// 启动定期上报
	go m.startPeriodicReporting()

	return nil
}

// startPeriodicReporting 启动定期上报
func (m *GRPCMonitorManager) startPeriodicReporting() {
	// 监控数据上报间隔
	var interval float64
	if flags.Interval <= 1 {
		interval = 1
	} else {
		interval = flags.Interval - 1
	}

	monitorTicker := time.NewTicker(time.Duration(interval * float64(time.Second)))
	defer monitorTicker.Stop()

	// 基础信息上报间隔
	basicInfoTicker := time.NewTicker(time.Duration(flags.InfoReportInterval) * time.Minute)
	defer basicInfoTicker.Stop()

	for {
		select {
		case <-monitorTicker.C:
			if err := m.sendMonitorReport(); err != nil {
				log.Printf("发送监控报告失败: %v", err)
				// 连接可能断开，尝试重新连接
				m.reconnect()
			}
		case <-basicInfoTicker.C:
			if err := m.sendBasicInfo(); err != nil {
				log.Printf("发送基础信息失败: %v", err)
			}
		}
	}
}

// sendMonitorReport 发送监控报告
func (m *GRPCMonitorManager) sendMonitorReport() error {
	// 生成监控数据
	monitoringData := monitoring.GenerateReport()

	// 转换为protobuf格式
	report, err := grpcClient.ConvertToMonitorReport(monitoringData, m.uuid)
	if err != nil {
		return err
	}

	// 发送到服务器
	return m.client.SendMonitorReport(report)
}

// sendBasicInfo 发送基础信息
func (m *GRPCMonitorManager) sendBasicInfo() error {
	// 转换基础信息为监控报告格式
	report := grpcClient.ConvertToBasicInfoReport(m.uuid)

	// 发送到服务器
	return m.client.SendMonitorReport(report)
}

// reconnect 重新连接
func (m *GRPCMonitorManager) reconnect() {
	log.Println("尝试重新连接gRPC服务器...")

	// 关闭当前连接
	m.client.Close()

	// 创建新的客户端
	m.client = grpcClient.NewGRPCClient()

	// 重试连接
	for retry := 0; retry <= flags.MaxRetries; retry++ {
		if retry > 0 {
			log.Printf("重连尝试 %d/%d", retry, flags.MaxRetries)
			time.Sleep(time.Duration(flags.ReconnectInterval) * time.Second)
		}

		if err := m.client.Connect(); err != nil {
			log.Printf("连接失败: %v", err)
			continue
		}

		if err := m.client.StartMonitorStream(); err != nil {
			log.Printf("启动监控流失败: %v", err)
			continue
		}

		log.Println("gRPC重连成功")
		return
	}

	log.Printf("达到最大重试次数 %d，重连失败", flags.MaxRetries)
}

// Stop 停止监控服务
func (m *GRPCMonitorManager) Stop() error {
	return m.client.Close()
}

// GetClient 获取gRPC客户端（供任务处理使用）
func (m *GRPCMonitorManager) GetClient() *grpcClient.GRPCClient {
	return m.client
}
