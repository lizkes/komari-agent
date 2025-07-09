package cmd

import (
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/komari-monitor/komari-agent/cmd/flags"
	"github.com/komari-monitor/komari-agent/logger"
	"github.com/komari-monitor/komari-agent/server"
	"github.com/komari-monitor/komari-agent/update"
	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:   "komari-agent",
	Short: "komari agent",
	Long:  `komari agent`,
	Run: func(cmd *cobra.Command, args []string) {
		// 设置日志级别
		logger.SetLevelFromString(flags.LogLevel)

		logger.Info("Komari Agent %s", update.CurrentVersion)
		logger.Info("Github Repo: %s", update.Repo)
		logger.Info("Log Level: %s", logger.GetLevelString())

		// 忽略不安全的证书
		if flags.IgnoreUnsafeCert {
			http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		}

		// 启动时立即上报一次基础信息
		logger.Info("Uploading basic info on startup...")
		server.UpdateBasicInfo()

		// 自动更新
		if !flags.DisableAutoUpdate {
			err := update.CheckAndUpdate()
			if err != nil {
				logger.Error("Auto update error: %v", err)
			}
			go update.DoUpdateWorks()
		}

		// 启动gRPC监控服务
		monitorManager := server.NewGRPCMonitorManager()
		for {
			if err := monitorManager.Start(); err != nil {
				logger.Error("启动gRPC监控服务失败: %v", err)
				logger.Info("等待 %d 秒后重试...", flags.ReconnectInterval)
				time.Sleep(time.Duration(flags.ReconnectInterval) * time.Second)
				continue
			}

			// gRPC服务启动成功，保持运行直到出错
			logger.Info("gRPC监控服务已启动，保持运行...")
			select {} // 阻塞主goroutine，让监控服务持续运行
		}
	},
}

func Execute() {
	for i, arg := range os.Args {
		if arg == "-autoUpdate" || arg == "--autoUpdate" {
			log.Println("WARNING: The -autoUpdate flag is deprecated in version 0.0.9 and later. Use --disable-auto-update to configure auto-update behavior.")
			// 从参数列表中移除该参数，防止cobra解析错误
			os.Args = append(os.Args[:i], os.Args[i+1:]...)
			break
		}
	}

	if err := RootCmd.Execute(); err != nil {
		logger.Error("Command execution error: %v", err)
	}
}

func init() {
	RootCmd.PersistentFlags().StringVarP(&flags.Token, "token", "t", "", "API token")
	RootCmd.MarkPersistentFlagRequired("token")
	RootCmd.PersistentFlags().StringVarP(&flags.Endpoint, "endpoint", "e", "", "API endpoint")
	RootCmd.MarkPersistentFlagRequired("endpoint")
	RootCmd.PersistentFlags().BoolVar(&flags.DisableAutoUpdate, "disable-auto-update", false, "Disable automatic updates")
	RootCmd.PersistentFlags().BoolVar(&flags.DisableWebSsh, "disable-web-ssh", false, "Disable remote control(web ssh and rce)")
	RootCmd.PersistentFlags().BoolVar(&flags.MemoryModeAvailable, "memory-mode-available", false, "Report memory as available instead of used.")
	RootCmd.PersistentFlags().Float64VarP(&flags.Interval, "interval", "i", 5.0, "Interval in seconds for general monitoring")
	RootCmd.PersistentFlags().BoolVarP(&flags.IgnoreUnsafeCert, "ignore-unsafe-cert", "u", false, "Ignore unsafe certificate errors")
	RootCmd.PersistentFlags().IntVarP(&flags.MaxRetries, "max-retries", "r", 3, "Maximum number of retries")
	RootCmd.PersistentFlags().IntVarP(&flags.ReconnectInterval, "reconnect-interval", "c", 5, "Reconnect interval in seconds")
	RootCmd.PersistentFlags().IntVar(&flags.InfoReportInterval, "info-report-interval", 30, "Interval in minutes for reporting basic info")
	RootCmd.PersistentFlags().Float64Var(&flags.NetworkInterval, "network-interval", 1.0, "Interval in seconds for network monitoring")
	RootCmd.PersistentFlags().StringVar(&flags.IncludeNics, "include-nics", "", "Comma-separated list of network interfaces to include")
	RootCmd.PersistentFlags().StringVar(&flags.ExcludeNics, "exclude-nics", "", "Comma-separated list of network interfaces to exclude")
	RootCmd.PersistentFlags().StringVar(&flags.LogLevel, "log-level", "info", "Log level (info, debug)")
	RootCmd.PersistentFlags().ParseErrorsWhitelist.UnknownFlags = true
}
