package grpc

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	ping "github.com/go-ping/ping"
	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/komari-monitor/komari-agent/cmd/flags"
	"github.com/komari-monitor/komari-agent/proto"
	"github.com/komari-monitor/komari-agent/terminal"
)

// GRPCClient gRPC客户端管理器
type GRPCClient struct {
	conn   *grpc.ClientConn
	client proto.MonitorServiceClient
	stream proto.MonitorService_StreamMonitorClient
	ctx    context.Context
	cancel context.CancelFunc
}

// NewGRPCClient 创建新的gRPC客户端
func NewGRPCClient() *GRPCClient {
	return &GRPCClient{}
}

// Connect 连接到gRPC服务器
func (c *GRPCClient) Connect() error {
	// 解析endpoint，构建gRPC地址
	grpcAddr := c.buildGRPCAddress()

	// 配置gRPC连接选项
	var opts []grpc.DialOption

	// 处理TLS证书验证
	if flags.IgnoreUnsafeCert {
		creds := credentials.NewTLS(&tls.Config{
			InsecureSkipVerify: true,
		})
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else if strings.HasPrefix(flags.Endpoint, "https://") {
		creds := credentials.NewTLS(&tls.Config{})
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// 连接超时
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, grpcAddr, opts...)
	if err != nil {
		return fmt.Errorf("连接gRPC服务器失败: %v", err)
	}

	c.conn = conn
	c.client = proto.NewMonitorServiceClient(conn)
	return nil
}

// StartMonitorStream 启动监控流
func (c *GRPCClient) StartMonitorStream() error {
	c.ctx, c.cancel = context.WithCancel(context.Background())

	stream, err := c.client.StreamMonitor(c.ctx)
	if err != nil {
		return fmt.Errorf("创建监控流失败: %v", err)
	}
	c.stream = stream

	// 发送认证请求
	authReq := &proto.MonitorRequest{
		Message: &proto.MonitorRequest_Auth{
			Auth: &proto.AuthRequest{
				Token: flags.Token,
			},
		},
	}

	if err := stream.Send(authReq); err != nil {
		return fmt.Errorf("发送认证请求失败: %v", err)
	}

	// 启动接收消息的goroutine
	go c.receiveMessages()

	return nil
}

// SendMonitorReport 发送监控报告
func (c *GRPCClient) SendMonitorReport(report *proto.MonitorReport) error {
	if c.stream == nil {
		return fmt.Errorf("监控流未建立")
	}

	req := &proto.MonitorRequest{
		Message: &proto.MonitorRequest_Report{
			Report: report,
		},
	}

	return c.stream.Send(req)
}

// SendPingResult 发送ping结果
func (c *GRPCClient) SendPingResult(result *proto.PingResult) error {
	if c.stream == nil {
		return fmt.Errorf("监控流未建立")
	}

	req := &proto.MonitorRequest{
		Message: &proto.MonitorRequest_PingResult{
			PingResult: result,
		},
	}

	return c.stream.Send(req)
}

// receiveMessages 接收服务器消息
func (c *GRPCClient) receiveMessages() {
	for {
		resp, err := c.stream.Recv()
		if err == io.EOF {
			log.Println("gRPC流已结束")
			return
		}
		if err != nil {
			log.Printf("接收gRPC消息错误: %v", err)
			return
		}

		c.handleServerMessage(resp)
	}
}

// handleServerMessage 处理服务器消息
func (c *GRPCClient) handleServerMessage(resp *proto.MonitorResponse) {
	switch msg := resp.Message.(type) {
	case *proto.MonitorResponse_AuthResponse:
		if msg.AuthResponse.Success {
			log.Println("gRPC认证成功")
		} else {
			log.Printf("gRPC认证失败: %s", msg.AuthResponse.ErrorMessage)
		}
	case *proto.MonitorResponse_TaskRequest:
		c.handleTaskRequest(msg.TaskRequest)
	case *proto.MonitorResponse_Error:
		log.Printf("服务器错误: %s", msg.Error.Error)
	default:
		log.Printf("未知消息类型: %T", msg)
	}
}

// handleTaskRequest 处理任务请求
func (c *GRPCClient) handleTaskRequest(taskReq *proto.TaskRequest) {
	switch task := taskReq.Task.(type) {
	case *proto.TaskRequest_Terminal:
		log.Printf("收到终端任务: %s", task.Terminal.RequestId)
		// 终端任务使用WebSocket连接
		go c.establishTerminalConnection(task.Terminal.RequestId)

	case *proto.TaskRequest_Exec:
		log.Printf("收到执行任务: %s, 命令: %s", task.Exec.TaskId, task.Exec.Command)
		go c.executeTask(task.Exec.TaskId, task.Exec.Command)

	case *proto.TaskRequest_Ping:
		log.Printf("收到ping任务: %d, 类型: %s, 目标: %s",
			task.Ping.PingTaskId, task.Ping.PingType, task.Ping.PingTarget)
		go c.executePingTask(task.Ping.PingTaskId, task.Ping.PingType, task.Ping.PingTarget)
	}
}

// establishTerminalConnection 建立终端WebSocket连接
func (c *GRPCClient) establishTerminalConnection(terminalID string) {
	endpoint := strings.TrimSuffix(flags.Endpoint, "/") + "/api/clients/terminal?token=" + flags.Token + "&id=" + terminalID
	endpoint = "ws" + strings.TrimPrefix(endpoint, "http")

	dialer := &websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}
	conn, _, err := dialer.Dial(endpoint, nil)
	if err != nil {
		log.Println("Failed to establish terminal connection:", err)
		return
	}

	// 启动终端
	terminal.StartTerminal(conn)
	if conn != nil {
		conn.Close()
	}
}

// buildGRPCAddress 构建gRPC服务器地址
func (c *GRPCClient) buildGRPCAddress() string {
	addr := flags.Endpoint
	var defaultPort string

	// 根据协议确定默认端口
	if strings.HasPrefix(addr, "https://") {
		defaultPort = "443"
		addr = strings.TrimPrefix(addr, "https://")
	} else if strings.HasPrefix(addr, "http://") {
		defaultPort = "80"
		addr = strings.TrimPrefix(addr, "http://")
	} else {
		defaultPort = "25775"
	}

	// 移除尾部斜杠
	addr = strings.TrimSuffix(addr, "/")

	// 检查是否已包含端口号
	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		// 没有端口号，添加对应协议的默认端口
		addr = net.JoinHostPort(addr, defaultPort)
	}

	return addr
}

// Close 关闭连接
func (c *GRPCClient) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	if c.stream != nil {
		c.stream.CloseSend()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// IsConnected 检查连接状态
func (c *GRPCClient) IsConnected() bool {
	return c.conn != nil && c.stream != nil
}

// executeTask 执行远程命令任务
func (c *GRPCClient) executeTask(taskID, command string) {
	if taskID == "" {
		return
	}
	if command == "" {
		log.Printf("任务 %s 没有提供命令", taskID)
		return
	}
	if flags.DisableWebSsh {
		log.Printf("远程执行功能已禁用，任务 %s", taskID)
		return
	}

	log.Printf("执行任务 %s: %s", taskID, command)

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; "+command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := stdout.String()
	if stderr.Len() > 0 {
		result += "\n" + stderr.String()
	}
	result = strings.ReplaceAll(result, "\r\n", "\n")

	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		}
	}

	log.Printf("任务 %s 执行完成，退出码: %d, 结果长度: %d", taskID, exitCode, len(result))

	// TODO: 通过gRPC发送执行结果
	// 目前protobuf没有定义执行结果类型，暂时通过日志记录
}

// executePingTask 执行ping任务
func (c *GRPCClient) executePingTask(taskID uint32, pingType, pingTarget string) {
	if taskID == 0 {
		log.Printf("无效的任务ID: %d", taskID)
		return
	}

	var err error
	var latency int64
	pingResult := int32(-1)
	timeout := 3 * time.Second

	switch pingType {
	case "icmp":
		if latency, err = c.icmpPing(pingTarget, timeout); err == nil {
			pingResult = int32(latency)
		}
	case "tcp":
		if latency, err = c.tcpPing(pingTarget, timeout); err == nil {
			pingResult = int32(latency)
		}
	case "http":
		if latency, err = c.httpPing(pingTarget, timeout); err == nil {
			pingResult = int32(latency)
		}
	default:
		log.Printf("不支持的ping类型: %s", pingType)
		return
	}

	if err != nil {
		log.Printf("Ping任务 %d 失败: %v", taskID, err)
		pingResult = -1
	}

	// 发送ping结果
	if pingResult != -1 {
		result := CreatePingResult(taskID, pingType, pingResult)
		if err := c.SendPingResult(result); err != nil {
			log.Printf("发送ping结果失败: %v", err)
		}
	}
}

// ping相关方法（从原始task.go移植）

// resolveIP 解析域名到 IP 地址，排除 DNS 查询时间
func (c *GRPCClient) resolveIP(target string) (string, error) {
	// 如果已经是 IP 地址，直接返回
	if ip := net.ParseIP(target); ip != nil {
		return target, nil
	}
	// 解析域名到 IP
	addrs, err := net.LookupHost(target)
	if err != nil || len(addrs) == 0 {
		return "", errors.New("failed to resolve target")
	}
	return addrs[0], nil // 返回第一个解析的 IP
}

// icmpPing ICMP ping实现
func (c *GRPCClient) icmpPing(target string, timeout time.Duration) (int64, error) {
	host, _, err := net.SplitHostPort(target)
	if err != nil {
		host = target
	}
	// For ICMP, we only need the host/IP, port is irrelevant.
	// If the host is an IPv6 literal, it might be wrapped in brackets.
	host = strings.Trim(host, "[]")

	// 先解析 IP 地址
	ip, err := c.resolveIP(host)
	if err != nil {
		return -1, err
	}

	pinger, err := ping.NewPinger(ip)
	if err != nil {
		return -1, err
	}
	pinger.Count = 1
	pinger.Timeout = timeout
	pinger.SetPrivileged(true)
	err = pinger.Run()
	if err != nil {
		return -1, err
	}
	stats := pinger.Statistics()
	if stats.PacketsRecv == 0 {
		return -1, errors.New("no packets received")
	}
	return stats.AvgRtt.Milliseconds(), nil
}

// tcpPing TCP ping实现
func (c *GRPCClient) tcpPing(target string, timeout time.Duration) (int64, error) {
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		// No port, assume port 80
		host = target
		port = "80"
	}

	ip, err := c.resolveIP(host)
	if err != nil {
		return -1, err
	}

	targetAddr := net.JoinHostPort(ip, port)
	start := time.Now()
	conn, err := net.DialTimeout("tcp", targetAddr, timeout)
	if err != nil {
		return -1, err
	}
	defer conn.Close()
	return time.Since(start).Milliseconds(), nil
}

// httpPing HTTP ping实现
func (c *GRPCClient) httpPing(target string, timeout time.Duration) (int64, error) {
	// Handle raw IPv6 address for URL
	if strings.Contains(target, ":") && !strings.Contains(target, "[") {
		// check if it's a valid IP to avoid wrapping hostnames
		if ip := net.ParseIP(target); ip != nil && ip.To4() == nil {
			target = "[" + target + "]"
		}
	}

	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "http://" + target
	}

	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				// 在 Dial 之前解析 IP，排除 DNS 时间
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				ip, err := c.resolveIP(host)
				if err != nil {
					return nil, err
				}
				return net.DialTimeout(network, net.JoinHostPort(ip, port), timeout)
			},
		},
	}
	start := time.Now()
	resp, err := client.Get(target)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return latency, nil
	}
	return latency, errors.New("http status not ok")
}
