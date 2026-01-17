package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"cfshare/internal/auth"
	"cfshare/internal/config"
	"cfshare/internal/server"
	"cfshare/internal/state"
	"cfshare/internal/tunnel"
)

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "__server__" {
		runServerProcess()
		return
	}

	var (
		publicMode bool
		password   string
		showHelp   bool
		forceStop  bool
		tunnelName string
		publicURL  string
		port       int
	)

	flag.BoolVar(&publicMode, "public", false, "公开分享（无需认证）")
	flag.StringVar(&password, "pass", "", "指定口令（默认随机生成）")
	flag.BoolVar(&showHelp, "help", false, "显示帮助")
	flag.BoolVar(&showHelp, "h", false, "显示帮助")
	flag.BoolVar(&forceStop, "force", false, "强制停止")
	flag.StringVar(&tunnelName, "tunnel", config.TunnelName, "Cloudflare Tunnel 名称")
	flag.StringVar(&publicURL, "url", "", "公开访问 URL（如 https://share.example.com）")
	flag.IntVar(&port, "port", config.DefaultPort, "本地监听端口")

	reorderArgs()
	flag.Parse()

	if showHelp {
		printUsage()
		return
	}

	args := flag.Args()

	if err := config.EnsureConfigDir(); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 无法创建配置目录: %v\n", err)
		os.Exit(1)
	}

	switch {
	case len(args) == 0:
		cmdStatus()

	case args[0] == "status":
		cmdStatus()

	case args[0] == "stop":
		cmdStop(forceStop)

	case args[0] == "setup":
		cmdSetup(tunnelName)

	case args[0] == "logs":
		cmdLogs()

	default:
		cmdShare(args[0], publicMode, password, port, tunnelName, publicURL)
	}
}

func printUsage() {
	fmt.Print(`cfshare - 通过 Cloudflare Tunnel 分享文件

用法:
    cfshare <path>              分享文件或目录（需要口令）
    cfshare <path> --public     公开分享（无需口令）
    cfshare <path> --pass xxx   使用指定口令
    cfshare                     查看当前分享状态
    cfshare status              查看详细状态
    cfshare stop                停止分享
    cfshare stop --force        强制停止
    cfshare setup               检查配置
    cfshare logs                查看访问日志

选项:
    --public        公开分享，无需认证
    --pass <pwd>    指定口令（默认随机生成）
    --port <port>   本地监听端口（默认 8787）
    --tunnel <n>    Cloudflare Tunnel 名称（默认 cfshare）
    --url <url>     公开访问 URL
    -h, --help      显示帮助

首次使用需要配置 Cloudflare Tunnel:
    1. 安装 cloudflared: brew install cloudflared
    2. 登录: cloudflared tunnel login
    3. 创建 tunnel: cloudflared tunnel create cfshare
    4. 配置 DNS: cloudflared tunnel route dns cfshare share.example.com
    5. 创建 ~/.cloudflared/config.yml

示例:
    cfshare ~/Documents/report.pdf
    cfshare ~/Pictures --public
    cfshare . --pass mypassword
`)
}

func cmdStatus() {
	st, err := state.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 读取状态失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(st.FormatStatus())
}

func cmdStop(force bool) {
	st, err := state.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 读取状态失败: %v\n", err)
		os.Exit(1)
	}

	if st == nil {
		fmt.Println("当前无活动分享")
		return
	}

	if st.ServerPID > 0 {
		stopProcess(st.ServerPID, force)
	}

	tm := tunnel.NewManager(config.TunnelName)
	if force {
		tm.ForceStop()
	} else {
		tm.Stop()
	}

	state.Clear()
	os.Remove(config.GetPidFilePath())

	fmt.Println("✅ 分享已停止")
}

func stopProcess(pid int, force bool) {
	process, err := os.FindProcess(pid)
	if err != nil {
		return
	}

	if force {
		process.Signal(syscall.SIGKILL)
	} else {
		process.Signal(syscall.SIGTERM)

		done := make(chan struct{})
		go func() {
			process.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(3 * time.Second):
			process.Signal(syscall.SIGKILL)
		}
	}
}

func cmdSetup(tunnelName string) {
	fmt.Println("检查 Cloudflare Tunnel 配置...")

	if err := tunnel.CheckSetup(tunnelName); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Cloudflare Tunnel 配置正确")

	tm := tunnel.NewManager(tunnelName)
	url, err := tm.GetPublicURL()
	if err != nil {
		fmt.Printf("⚠️  无法获取公开 URL: %v\n", err)
		fmt.Println("   请在运行 cfshare 时使用 --url 参数指定")
	} else {
		fmt.Printf("   公开 URL: %s\n", url)
	}
}

func cmdLogs() {
	logPath := config.GetAccessLogPath()
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("暂无访问日志")
			return
		}
		fmt.Fprintf(os.Stderr, "错误: 读取日志失败: %v\n", err)
		os.Exit(1)
	}

	lines := strings.Split(string(data), "\n")
	start := 0
	if len(lines) > 20 {
		start = len(lines) - 20
	}

	fmt.Println("最近的访问日志:")
	fmt.Println("─────────────────────────────────────────")
	for _, line := range lines[start:] {
		if line != "" {
			fmt.Println(line)
		}
	}
}

func cmdShare(path string, public bool, password string, port int, tunnelName, publicURL string) {
	if _, err := os.Stat(path); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 路径不存在: %s\n", path)
		os.Exit(1)
	}

	existingState, _ := state.Load()
	if existingState != nil && existingState.IsRunning() {
		fmt.Println("正在停止现有分享...")
		cmdStop(false)
		time.Sleep(500 * time.Millisecond)
	}

	username := ""
	if !public {
		username = config.DefaultUsername
		if password == "" {
			password = auth.GeneratePassword(config.PasswordLength)
		}
	}

	if publicURL == "" {
		tm := tunnel.NewManager(tunnelName)
		var err error
		publicURL, err = tm.GetPublicURL()
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: 无法获取公开 URL: %v\n", err)
			fmt.Fprintln(os.Stderr, "请使用 --url 参数指定公开 URL")
			os.Exit(1)
		}
	}

	st := &state.State{
		ShareID:   fmt.Sprintf("%d", time.Now().Unix()),
		Port:      port,
		StartTime: time.Now(),
		PublicURL: publicURL,
	}

	if public {
		st.Mode = state.ModePublic
	} else {
		st.Mode = state.ModeProtected
		st.Username = username
		st.Password = password
	}

	serverPID, err := startServerProcess(path, port, username, password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 启动服务器失败: %v\n", err)
		os.Exit(1)
	}
	st.ServerPID = serverPID

	tm := tunnel.NewManager(tunnelName)
	tunnelPID, err := tm.Start()
	if err != nil {
		stopProcess(serverPID, true)
		fmt.Fprintf(os.Stderr, "错误: 启动 tunnel 失败: %v\n", err)
		os.Exit(1)
	}
	st.TunnelPID = tunnelPID

	absPath, _ := filepath.Abs(path)
	fi, _ := os.Stat(absPath)
	st.Path = absPath
	if fi.IsDir() {
		st.ShareType = state.TypeDir
	} else {
		st.ShareType = state.TypeFile
	}

	if err := st.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 保存状态失败: %v\n", err)
	}

	fmt.Print(st.FormatShareOutput())
}

func startServerProcess(path string, port int, username, password string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("get executable: %w", err)
	}

	args := []string{"__server__", path, strconv.Itoa(port), username, password}
	cmd := exec.Command(exe, args...)

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	logPath := config.GetConfigDir() + "/server.log"
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return 0, fmt.Errorf("create log file: %w", err)
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return 0, fmt.Errorf("start server: %w", err)
	}

	pid := cmd.Process.Pid

	os.WriteFile(config.GetPidFilePath(), []byte(strconv.Itoa(pid)), 0600)

	time.Sleep(300 * time.Millisecond)

	return pid, nil
}

func runServerProcess() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "invalid server arguments")
		os.Exit(1)
	}

	path := os.Args[2]
	port, _ := strconv.Atoi(os.Args[3])
	username := ""
	password := ""
	if len(os.Args) >= 6 {
		username = os.Args[4]
		password = os.Args[5]
	}

	st, err := state.Load()
	if err != nil || st == nil {
		st = &state.State{}
	}

	srv, err := server.NewServer(path, st)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create server: %v\n", err)
		os.Exit(1)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigChan
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
		os.Exit(0)
	}()

	fmt.Printf("Starting server on port %d for path: %s\n", port, path)
	if err := srv.Start(port, username, password); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

// reorderArgs 重排参数，让 flags 在位置参数之前
func reorderArgs() {
	if len(os.Args) <= 2 {
		return
	}

	var flags []string
	var positional []string

	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			// 如果是带值的 flag，把值也加进去
			if (arg == "--pass" || arg == "--port" || arg == "--tunnel" || arg == "--url") && i+1 < len(os.Args) {
				i++
				flags = append(flags, os.Args[i])
			}
		} else {
			positional = append(positional, arg)
		}
	}

	newArgs := []string{os.Args[0]}
	newArgs = append(newArgs, flags...)
	newArgs = append(newArgs, positional...)
	os.Args = newArgs
}
