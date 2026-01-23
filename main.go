package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "__server__" {
		runServerProcess()
		return
	}

	var (
		publicMode     bool
		password       string
		showHelp       bool
		showHelpChinese bool
		showVersion    bool
		forceStop      bool
		tunnelName     string
		publicURL      string
		port           int
	)

	flag.BoolVar(&publicMode, "public", false, "Public share (no authentication)")
	flag.StringVar(&password, "pass", "", "Specify password (default: random)")
	flag.BoolVar(&showHelp, "help", false, "Show help")
	flag.BoolVar(&showHelp, "h", false, "Show help")
	flag.BoolVar(&showHelpChinese, "hc", false, "Show help in Chinese")
	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.BoolVar(&showVersion, "v", false, "Show version")
	flag.BoolVar(&forceStop, "force", false, "Force stop")
	flag.StringVar(&tunnelName, "tunnel", config.TunnelName, "Cloudflare Tunnel name")
	flag.StringVar(&publicURL, "url", "", "Public access URL")
	flag.IntVar(&port, "port", config.DefaultPort, "Local listen port")

	reorderArgs()
	flag.Parse()

	if showHelp {
		printUsage()
		return
	}

	if showHelpChinese {
		printUsageChinese()
		return
	}

	if showVersion {
		fmt.Printf("cfshare %s (commit: %s, built: %s)\n", version, commit, date)
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

	case args[0] == "add":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "用法: cfshare add <path>...")
			os.Exit(1)
		}
		cmdAdd(args[1:])

	case args[0] == "rm" || args[0] == "remove":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "用法: cfshare rm <name>...")
			os.Exit(1)
		}
		cmdRemove(args[1:])

	default:
		cmdShare(args, publicMode, password, port, tunnelName, publicURL)
	}
}

func printUsage() {
	fmt.Print(`cfshare - Share files via Cloudflare Tunnel

Usage:
    cfshare <path>...           Share file(s)/directory (password protected)
    cfshare <path>... --public  Share publicly (no authentication)
    cfshare <path>... --pass x  Share with specified password
    cfshare                     Show current share status
    cfshare status              Show detailed status
    cfshare add <path>...       Add file(s)/directory to current share
    cfshare rm <name>...        Remove item(s) from current share
    cfshare stop                Stop sharing
    cfshare stop --force        Force stop
    cfshare setup               Check configuration
    cfshare logs                View access logs

Options:
    --public        Public share, no authentication required
    --pass <pwd>    Specify password (default: randomly generated)
    --port <port>   Local listen port (default: 8787)
    --tunnel <n>    Cloudflare Tunnel name (default: cfshare)
    --url <url>     Public access URL
    -h, --help      Show help (English)
    -hc             Show help (Chinese)
    -v, --version   Show version

First-time setup requires Cloudflare Tunnel configuration:
    1. Install cloudflared: brew install cloudflared
    2. Login: cloudflared tunnel login
    3. Create tunnel: cloudflared tunnel create cfshare
    4. Configure DNS: cloudflared tunnel route dns cfshare share.example.com
    5. Create ~/.cloudflared/config.yml

Examples:
    cfshare ~/Documents/report.pdf
    cfshare ~/Pictures --public
    cfshare . --pass mypassword
    cfshare file1.pdf file2.txt dir1/    # Multi-file share
    cfshare add newfile.txt              # Dynamically add file
    cfshare rm oldfile.txt               # Dynamically remove file
`)
}

func printUsageChinese() {
	fmt.Print(`cfshare - 通过 Cloudflare Tunnel 分享文件

用法:
    cfshare <path>...           分享一个或多个文件/目录（需要口令）
    cfshare <path>... --public  公开分享（无需口令）
    cfshare <path>... --pass x  使用指定口令
    cfshare                     查看当前分享状态
    cfshare status              查看详细状态
    cfshare add <path>...       添加文件/目录到当前分享
    cfshare rm <name>...        从当前分享中移除项目
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
    -h, --help      显示帮助（英文）
    -hc             显示帮助（中文）
    -v, --version   显示版本

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
    cfshare file1.pdf file2.txt dir1/    # 多文件分享
    cfshare add newfile.txt              # 动态添加文件
    cfshare rm oldfile.txt               # 动态移除文件
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

func cmdAdd(paths []string) {
	st, err := state.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 读取状态失败: %v\n", err)
		os.Exit(1)
	}

	if st == nil || !st.IsRunning() {
		fmt.Fprintln(os.Stderr, "错误: 当前没有活动的分享")
		fmt.Fprintln(os.Stderr, "请先使用 cfshare <path>... 启动分享")
		os.Exit(1)
	}

	// 构建现有名称集合
	existingNames := make(map[string]bool)
	for _, item := range st.Items {
		existingNames[item.Name] = true
	}

	// 验证并添加新路径
	var newItems []state.ShareItem
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			fmt.Fprintf(os.Stderr, "错误: 路径不存在: %s\n", path)
			os.Exit(1)
		}

		absPath, _ := filepath.Abs(path)
		name := filepath.Base(absPath)

		// 检查名称冲突
		if existingNames[name] {
			fmt.Fprintf(os.Stderr, "错误: 名称 '%s' 已存在\n", name)
			os.Exit(1)
		}

		fi, _ := os.Stat(absPath)
		item := state.ShareItem{
			Path: absPath,
			Name: name,
		}
		if fi.IsDir() {
			item.ShareType = state.TypeDir
			item.Size = 0
		} else {
			item.ShareType = state.TypeFile
			item.Size = fi.Size()
		}

		newItems = append(newItems, item)
		existingNames[name] = true
	}

	// 更新状态
	st.Items = append(st.Items, newItems...)
	st.IsMulti = len(st.Items) > 1

	if err := st.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 保存状态失败: %v\n", err)
		os.Exit(1)
	}

	// 重启服务器以加载新配置
	restartServer(st)

	fmt.Printf("✅ 已添加 %d 个项目\n", len(newItems))
	for _, item := range newItems {
		fmt.Printf("  + %s (%s)\n", item.Name, item.ShareType)
	}
	fmt.Printf("\n当前共 %d 个分享项\n", len(st.Items))
}

func cmdRemove(names []string) {
	st, err := state.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 读取状态失败: %v\n", err)
		os.Exit(1)
	}

	if st == nil || !st.IsRunning() {
		fmt.Fprintln(os.Stderr, "错误: 当前没有活动的分享")
		os.Exit(1)
	}

	if len(st.Items) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 当前没有分享项")
		os.Exit(1)
	}

	// 构建要删除的名称集合
	toRemove := make(map[string]bool)
	for _, name := range names {
		toRemove[name] = true
	}

	// 过滤保留的项
	var remaining []state.ShareItem
	var removed []string
	for _, item := range st.Items {
		if toRemove[item.Name] {
			removed = append(removed, item.Name)
		} else {
			remaining = append(remaining, item)
		}
	}

	if len(removed) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 未找到指定的项目")
		fmt.Println("当前分享的项目:")
		for _, item := range st.Items {
			fmt.Printf("  - %s\n", item.Name)
		}
		os.Exit(1)
	}

	if len(remaining) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 不能删除所有项目")
		fmt.Fprintln(os.Stderr, "如需停止分享，请使用 cfshare stop")
		os.Exit(1)
	}

	// 更新状态
	st.Items = remaining
	st.IsMulti = len(st.Items) > 1

	// 单文件时更新兼容字段
	if len(st.Items) == 1 {
		st.Path = st.Items[0].Path
		st.ShareType = st.Items[0].ShareType
	}

	if err := st.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 保存状态失败: %v\n", err)
		os.Exit(1)
	}

	// 重启服务器以加载新配置
	restartServer(st)

	fmt.Printf("✅ 已移除 %d 个项目\n", len(removed))
	for _, name := range removed {
		fmt.Printf("  - %s\n", name)
	}
	fmt.Printf("\n剩余 %d 个分享项\n", len(st.Items))
}

func restartServer(st *state.State) {
	// 停止旧服务器
	if st.ServerPID > 0 {
		stopProcess(st.ServerPID, false)
		time.Sleep(300 * time.Millisecond)
	}

	// 收集路径
	var paths []string
	for _, item := range st.Items {
		paths = append(paths, item.Path)
	}

	// 重新启动服务器
	username := st.Username
	password := st.Password

	serverPID, err := startServerProcess(paths, st.Port, username, password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 重启服务器失败: %v\n", err)
		os.Exit(1)
	}

	st.ServerPID = serverPID
	st.Save()
}

func cmdShare(paths []string, public bool, password string, port int, tunnelName, publicURL string) {
	// 验证所有路径存在
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			fmt.Fprintf(os.Stderr, "错误: 路径不存在: %s\n", path)
			os.Exit(1)
		}
	}

	// 检查名称冲突
	names := make(map[string]string)
	for _, path := range paths {
		absPath, _ := filepath.Abs(path)
		name := filepath.Base(absPath)
		if existing, ok := names[name]; ok {
			fmt.Fprintf(os.Stderr, "错误: 名称冲突: '%s'\n", name)
			fmt.Fprintf(os.Stderr, "  - %s\n", existing)
			fmt.Fprintf(os.Stderr, "  - %s\n", absPath)
			fmt.Fprintln(os.Stderr, "请重命名文件后再试")
			os.Exit(1)
		}
		names[name] = absPath
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

	serverPID, err := startServerProcess(paths, port, username, password)
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

	// 构建 Items 列表
	var items []state.ShareItem
	for _, path := range paths {
		absPath, _ := filepath.Abs(path)
		fi, _ := os.Stat(absPath)
		item := state.ShareItem{
			Path: absPath,
			Name: filepath.Base(absPath),
		}
		if fi.IsDir() {
			item.ShareType = state.TypeDir
			item.Size = 0
		} else {
			item.ShareType = state.TypeFile
			item.Size = fi.Size()
		}
		items = append(items, item)
	}

	st.Items = items
	st.IsMulti = len(items) > 1

	// 单文件时填充兼容字段
	if len(items) == 1 {
		st.Path = items[0].Path
		st.ShareType = items[0].ShareType
	}

	if err := st.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 保存状态失败: %v\n", err)
	}

	fmt.Print(st.FormatShareOutput())
}

func startServerProcess(paths []string, port int, username, password string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("get executable: %w", err)
	}

	// 使用 JSON + base64 编码传递多路径
	pathsJSON, _ := json.Marshal(paths)
	pathsArg := base64.StdEncoding.EncodeToString(pathsJSON)
	args := []string{"__server__", pathsArg, strconv.Itoa(port), username, password}
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

	// 解析 JSON + base64 编码的多路径
	pathsArg := os.Args[2]
	decoded, err := base64.StdEncoding.DecodeString(pathsArg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decode paths: %v\n", err)
		os.Exit(1)
	}
	var paths []string
	json.Unmarshal(decoded, &paths)
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

	srv, err := server.NewServer(paths, st)
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

	fmt.Printf("Starting server on port %d for paths: %v\n", port, paths)
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
