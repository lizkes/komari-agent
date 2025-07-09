package server

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/komari-monitor/komari-agent/cmd/flags"
	grpcClient "github.com/komari-monitor/komari-agent/grpc"
	"github.com/komari-monitor/komari-agent/logger"
	"github.com/komari-monitor/komari-agent/monitoring"
)

// GRPCMonitorManager gRPC监控管理器
type GRPCMonitorManager struct {
	client          *grpcClient.GRPCClient
	uuid            string
	reconnectCount  int           // 重连计数器
	lastConnectTime time.Time     // 上次连接时间
	totalUptime     time.Duration // 总运行时间
	startTime       time.Time     // 启动时间
}

// NewGRPCMonitorManager 创建新的gRPC监控管理器
func NewGRPCMonitorManager() *GRPCMonitorManager {
	return &GRPCMonitorManager{
		client:         grpcClient.NewGRPCClient(),
		uuid:           uuid.New().String(),
		reconnectCount: 0,
		startTime:      time.Now(),
	}
}

// Start 启动gRPC监控服务
func (m *GRPCMonitorManager) Start() error {
	logger.Info("启动gRPC监控服务")
	logger.ConnectionDiag("监控管理器启动: UUID=%s", m.uuid)

	// 连接到gRPC服务器
	connectStartTime := time.Now()
	if err := m.client.Connect(); err != nil {
		logger.ConnectionDiag("gRPC连接失败: %v", err)
		return err
	}
	m.lastConnectTime = time.Now()
	connectDuration := time.Since(connectStartTime)
	logger.ConnectionDiag("gRPC连接建立耗时: %v", connectDuration)

	// 启动监控流
	streamStartTime := time.Now()
	if err := m.client.StartMonitorStream(); err != nil {
		logger.ConnectionDiag("监控流启动失败: %v", err)
		return err
	}
	streamDuration := time.Since(streamStartTime)
	logger.ConnectionDiag("监控流启动耗时: %v", streamDuration)

	logger.Info("gRPC监控服务已启动")

	// 首次发送基础信息
	if err := m.sendBasicInfo(); err != nil {
		logger.Error("发送基础信息失败: %v", err)
	}

	// 启动错误监听
	go m.startErrorMonitoring()

	// 启动定期上报
	go m.startPeriodicReporting()

	return nil
}

// startErrorMonitoring 启动错误监听，处理连接断开和重连
func (m *GRPCMonitorManager) startErrorMonitoring() {
	logger.ConnectionDiag("错误监听器已启动")

	for {
		err := <-m.client.GetErrorChannel()
		if err != nil {
			// 计算连接持续时间
			connectionDuration := time.Since(m.lastConnectTime)
			m.reconnectCount++

			// 分析错误类型
			errorType := m.analyzeError(err)

			logger.ConnectionDiag("连接错误检测: 类型=%s, 连接持续时间=%v, 重连次数=%d, 错误=%v",
				errorType, connectionDuration, m.reconnectCount, err)
			logger.Error("gRPC连接出现错误: %v，开始重连...", err)

			m.reconnect()
		}
	}
}

// analyzeError 分析错误类型，用于WARP路由问题诊断
func (m *GRPCMonitorManager) analyzeError(err error) string {
	errStr := err.Error()

	// 常见网络错误分类
	switch {
	case strings.Contains(errStr, "connection reset"):
		return "CONNECTION_RESET"
	case strings.Contains(errStr, "timeout"):
		return "TIMEOUT"
	case strings.Contains(errStr, "context canceled"):
		return "CONTEXT_CANCELED"
	case strings.Contains(errStr, "broken pipe"):
		return "BROKEN_PIPE"
	case strings.Contains(errStr, "network is unreachable"):
		return "NETWORK_UNREACHABLE"
	case strings.Contains(errStr, "no route to host"):
		return "NO_ROUTE"
	case strings.Contains(errStr, "connection refused"):
		return "CONNECTION_REFUSED"
	case strings.Contains(errStr, "EOF"):
		return "EOF"
	default:
		return "UNKNOWN"
	}
}

// startPeriodicReporting 启动定期上报（差异化频率）
func (m *GRPCMonitorManager) startPeriodicReporting() {
	logger.ConnectionDiag("定期上报器已启动: 网络间隔=%.1fs, 常规间隔=%.1fs, 基础信息间隔=%dm",
		flags.NetworkInterval, flags.Interval, flags.InfoReportInterval)

	// 网络数据定时器（每flags.NetworkInterval秒）
	networkTicker := time.NewTicker(time.Duration(flags.NetworkInterval) * time.Second)
	defer networkTicker.Stop()

	// 常规监控数据定时器（每flags.Interval秒）
	generalTicker := time.NewTicker(time.Duration(flags.Interval) * time.Second)
	defer generalTicker.Stop()

	// 基础信息上报定时器（每flags.InfoReportInterval分钟）
	basicInfoTicker := time.NewTicker(time.Duration(flags.InfoReportInterval) * time.Minute)
	defer basicInfoTicker.Stop()

	networkReportCount := 0
	generalReportCount := 0

	for {
		select {
		case <-networkTicker.C:
			networkReportCount++
			logger.ConnectionDiag("发送网络报告 #%d", networkReportCount)
			if err := m.sendNetworkReport(); err != nil {
				logger.Error("发送网络报告失败: %v", err)
				// 错误监听器会自动处理重连，这里不需要手动调用
			}
		case <-generalTicker.C:
			generalReportCount++
			logger.ConnectionDiag("发送常规报告 #%d", generalReportCount)
			if err := m.sendGeneralReport(); err != nil {
				logger.Error("发送常规报告失败: %v", err)
				// 错误监听器会自动处理重连，这里不需要手动调用
			}
		case <-basicInfoTicker.C:
			logger.ConnectionDiag("发送基础信息")
			if err := m.sendBasicInfo(); err != nil {
				logger.Error("发送基础信息失败: %v", err)
			}
		}
	}
}

// sendNetworkReport 发送网络监控报告
func (m *GRPCMonitorManager) sendNetworkReport() error {
	// 生成网络监控数据
	networkData := monitoring.GenerateNetworkReport()

	// 转换为protobuf格式
	report, err := grpcClient.ConvertToMonitorReport(networkData, m.uuid)
	if err != nil {
		return err
	}

	// 发送到服务器
	return m.client.SendMonitorReport(report)
}

// sendGeneralReport 发送常规监控报告
func (m *GRPCMonitorManager) sendGeneralReport() error {
	// 生成常规监控数据
	generalData := monitoring.GenerateGeneralReport()

	// 转换为protobuf格式
	report, err := grpcClient.ConvertToMonitorReport(generalData, m.uuid)
	if err != nil {
		return err
	}

	// 发送到服务器
	return m.client.SendMonitorReport(report)
}

// sendMonitorReport 发送监控报告（保留兼容性）
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
	logger.ConnectionDiag("开始重连流程: 第%d次重连", m.reconnectCount)
	logger.Info("尝试重新连接gRPC服务器...")

	// 关闭当前连接
	m.client.Close()

	// 创建新的客户端
	m.client = grpcClient.NewGRPCClient()

	// 重试连接
	for retry := 0; retry <= flags.MaxRetries; retry++ {
		if retry > 0 {
			logger.Info("重连尝试 %d/%d", retry, flags.MaxRetries)
			logger.ConnectionDiag("等待重连间隔: %ds", flags.ReconnectInterval)
			time.Sleep(time.Duration(flags.ReconnectInterval) * time.Second)
		}

		logger.ConnectionDiag("尝试建立连接: 第%d次", retry+1)
		if err := m.client.Connect(); err != nil {
			logger.ConnectionDiag("连接失败: %v", err)
			logger.Error("连接失败: %v", err)
			continue
		}

		logger.ConnectionDiag("连接成功，尝试启动监控流")
		if err := m.client.StartMonitorStream(); err != nil {
			logger.ConnectionDiag("监控流启动失败: %v", err)
			logger.Error("启动监控流失败: %v", err)
			continue
		}

		// 重连成功
		m.lastConnectTime = time.Now()
		totalUptime := time.Since(m.startTime)
		logger.ConnectionDiag("重连成功: 总运行时间=%v, 总重连次数=%d", totalUptime, m.reconnectCount)
		logger.Info("gRPC重连成功")

		// 重连成功后，重新启动错误监听
		go m.startErrorMonitoring()

		return
	}

	logger.ConnectionDiag("重连失败: 达到最大重试次数%d", flags.MaxRetries)
	logger.Error("达到最大重试次数 %d，重连失败", flags.MaxRetries)
}

// Stop 停止监控服务
func (m *GRPCMonitorManager) Stop() error {
	return m.client.Close()
}

// GetClient 获取gRPC客户端（供任务处理使用）
func (m *GRPCMonitorManager) GetClient() *grpcClient.GRPCClient {
	return m.client
}
