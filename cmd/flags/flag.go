package flags

var (
	DisableAutoUpdate   bool
	DisableWebSsh       bool
	MemoryModeAvailable bool
	Token               string
	Endpoint            string
	Interval            float64
	IgnoreUnsafeCert    bool
	MaxRetries          int
	ReconnectInterval   int
	InfoReportInterval  int
	NetworkInterval     float64 // 新增：网络监控间隔
	IncludeNics         string
	ExcludeNics         string
)
