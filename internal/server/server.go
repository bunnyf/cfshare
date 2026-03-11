package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"cfshare/internal/auth"
	"cfshare/internal/config"
	"cfshare/internal/state"
)

type Server struct {
	// 多路径支持
	items   []state.ShareItem
	itemMap map[string]*state.ShareItem // 名称->项映射
	isMulti bool

	// 单文件兼容
	sharePath string
	shareType state.ShareType

	state   *state.State
	stateMu sync.Mutex
	srv     *http.Server
}

func NewServer(paths []string, st *state.State) (*Server, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no paths provided")
	}

	var items []state.ShareItem

	for _, p := range paths {
		absPath, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("invalid path %s: %w", p, err)
		}

		info, err := os.Stat(absPath)
		if err != nil {
			return nil, fmt.Errorf("cannot access %s: %w", p, err)
		}

		item := state.ShareItem{
			Path: absPath,
			Name: filepath.Base(absPath),
		}

		if info.IsDir() {
			item.ShareType = state.TypeDir
			item.Size = 0
		} else {
			item.ShareType = state.TypeFile
			item.Size = info.Size()
		}

		items = append(items, item)
	}

	// 检测名称冲突
	itemMap, err := buildItemMap(items)
	if err != nil {
		return nil, err
	}

	// 单路径: 保持向后兼容
	if len(items) == 1 {
		st.Items = items
		st.Path = items[0].Path
		st.ShareType = items[0].ShareType
		st.IsMulti = false

		return &Server{
			sharePath: items[0].Path,
			shareType: items[0].ShareType,
			items:     items,
			itemMap:   itemMap,
			isMulti:   false,
			state:     st,
		}, nil
	}

	// 多路径
	st.Items = items
	st.IsMulti = true

	return &Server{
		items:   items,
		itemMap: itemMap,
		isMulti: true,
		state:   st,
	}, nil
}

// buildItemMap 构建名称到项的映射，检测名称冲突
func buildItemMap(items []state.ShareItem) (map[string]*state.ShareItem, error) {
	result := make(map[string]*state.ShareItem)

	for i := range items {
		name := items[i].Name
		if _, exists := result[name]; exists {
			return nil, fmt.Errorf("名称冲突: 多个分享项具有相同名称 '%s'，请重命名后再试", name)
		}
		result[name] = &items[i]
	}

	return result, nil
}

func (s *Server) Start(port int, username, password string) error {
	mux := http.NewServeMux()

	var handler http.Handler = http.HandlerFunc(s.handleRequest)
	handler = s.loggingMiddleware(handler)

	if username != "" && password != "" {
		handler = auth.BasicAuthMiddleware(username, password, handler)
	}

	mux.Handle("/", handler)

	s.srv = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	return s.srv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.srv != nil {
		return s.srv.Shutdown(ctx)
	}
	return nil
}

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Cache-Control is set per response type:
	// - File downloads: allow Cloudflare CDN caching (public, max-age=3600)
	// - Directory listings: no-store (dynamic HTML)

	if !s.isMulti {
		// 向后兼容: 单路径模式
		if s.shareType == state.TypeFile {
			s.serveFile(w, r)
		} else {
			s.serveDir(w, r)
		}
		return
	}

	// 多文件模式
	s.handleMultiShare(w, r)
}

// handleMultiShare 处理多文件分享请求
func (s *Server) handleMultiShare(w http.ResponseWriter, r *http.Request) {
	reqPath := strings.TrimPrefix(filepath.Clean(r.URL.Path), "/")

	// 根路径: 显示虚拟目录列表
	if reqPath == "/" || reqPath == "." || reqPath == "" {
		s.listVirtualRoot(w, r)
		return
	}

	// 解析第一级路径名
	trimmedPath := strings.TrimPrefix(reqPath, "/")
	parts := strings.SplitN(trimmedPath, "/", 2)
	itemName := parts[0]
	subPath := ""
	if len(parts) > 1 {
		subPath = parts[1]
	}

	// 查找分享项
	item, ok := s.itemMap[itemName]
	if !ok {
		http.NotFound(w, r)
		return
	}

	// 根据分享项类型处理
	if item.ShareType == state.TypeFile {
		// 文件: 直接下载 (忽略 subPath)
		if subPath != "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, item.Name))
		http.ServeFile(w, r, item.Path)
	} else {
		// 目录: 使用基于项的目录浏览
		s.serveDirWithBase(w, r, item.Path, "/"+itemName, subPath)
	}
}

// listVirtualRoot 列出虚拟根目录（所有分享项）
func (s *Server) listVirtualRoot(w http.ResponseWriter, r *http.Request) {
	var files []FileInfo

	for _, item := range s.items {
		fi := FileInfo{
			Name:  item.Name,
			Size:  item.Size,
			IsDir: item.ShareType == state.TypeDir,
			Path:  "/" + item.Name,
		}
		if fi.IsDir {
			fi.Path += "/"
		}
		// 获取真实的修改时间
		if info, err := os.Stat(item.Path); err == nil {
			fi.ModTime = info.ModTime()
		} else {
			fi.ModTime = time.Now()
		}
		files = append(files, fi)
	}

	// 排序: 目录在前，文件在后，按名称排序
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return files[i].Name < files[j].Name
	})

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	tmpl := template.Must(template.New("dir").Funcs(template.FuncMap{
		"formatSize": formatSize,
		"formatTime": func(t time.Time) string { return t.Format("2006-01-02 15:04") },
	}).Parse(dirTemplate))

	data := struct {
		Path   string
		Files  []FileInfo
		Parent string
	}{
		Path:   "/",
		Files:  files,
		Parent: "",
	}

	tmpl.Execute(w, data)
}

// serveDirWithBase 处理多文件模式下的目录浏览
func (s *Server) serveDirWithBase(w http.ResponseWriter, r *http.Request, basePath, urlPrefix, subPath string) {
	// 清理子路径
	cleanSub := filepath.Clean(subPath)
	if cleanSub == "." {
		cleanSub = ""
	}
	if strings.HasPrefix(cleanSub, "..") {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// 解析 basePath 的真实路径（处理 /tmp -> /private/tmp 等情况）
	realBasePath, err := filepath.EvalSymlinks(basePath)
	if err != nil {
		realBasePath = basePath
	}

	fullPath := realBasePath
	if cleanSub != "" {
		fullPath = filepath.Join(realBasePath, cleanSub)
	}

	// 防止路径遍历
	if !strings.HasPrefix(fullPath, realBasePath) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// 检查符号链接是否指向 basePath 外部
	realFullPath, err := filepath.EvalSymlinks(fullPath)
	if err == nil && !strings.HasPrefix(realFullPath, realBasePath) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
		} else {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	if info.IsDir() {
		s.listDirectoryWithBase(w, r, fullPath, urlPrefix, subPath)
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(fullPath)))
		http.ServeFile(w, r, fullPath)
	}
}

// listDirectoryWithBase 列出目录内容（多文件模式）
func (s *Server) listDirectoryWithBase(w http.ResponseWriter, r *http.Request, fullPath, urlPrefix, subPath string) {
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var files []FileInfo
	currentPath := urlPrefix
	if subPath != "" {
		currentPath = urlPrefix + "/" + subPath
	}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		entryPath := currentPath + "/" + entry.Name()
		if entry.IsDir() {
			entryPath += "/"
		}

		files = append(files, FileInfo{
			Name:    entry.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			IsDir:   entry.IsDir(),
			Path:    entryPath,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return files[i].Name < files[j].Name
	})

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	tmpl := template.Must(template.New("dir").Funcs(template.FuncMap{
		"formatSize": formatSize,
		"formatTime": func(t time.Time) string { return t.Format("2006-01-02 15:04") },
	}).Parse(dirTemplate))

	// 计算父目录
	parent := "/"
	if subPath != "" {
		parent = urlPrefix + "/" + filepath.Dir(subPath)
		if parent == urlPrefix+"/." {
			parent = urlPrefix
		}
	}

	displayPath := currentPath
	if !strings.HasSuffix(displayPath, "/") {
		displayPath += "/"
	}

	data := struct {
		Path   string
		Files  []FileInfo
		Parent string
	}{
		Path:   displayPath,
		Files:  files,
		Parent: parent,
	}

	tmpl.Execute(w, data)
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request) {
	reqPath := r.URL.Path
	fileName := filepath.Base(s.sharePath)

	if reqPath != "/" && reqPath != "/"+fileName {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	http.ServeFile(w, r, s.sharePath)
}

func (s *Server) serveDir(w http.ResponseWriter, r *http.Request) {
	reqPath := strings.TrimPrefix(filepath.Clean(r.URL.Path), "/")
	if strings.HasPrefix(reqPath, "..") {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	fullPath := filepath.Join(s.sharePath, reqPath)

	if !strings.HasPrefix(fullPath, s.sharePath) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	realPath, err := filepath.EvalSymlinks(fullPath)
	if err == nil && !strings.HasPrefix(realPath, s.sharePath) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
		} else {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	if info.IsDir() {
		s.listDirectory(w, r, fullPath, reqPath)
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(fullPath)))
		http.ServeFile(w, r, fullPath)
	}
}

type FileInfo struct {
	Name    string
	Size    int64
	ModTime time.Time
	IsDir   bool
	Path    string
}

func (s *Server) listDirectory(w http.ResponseWriter, r *http.Request, fullPath, reqPath string) {
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "no-store")

	var files []FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		entryPath := filepath.Join(reqPath, entry.Name())
		if entry.IsDir() {
			entryPath += "/"
		}

		files = append(files, FileInfo{
			Name:    entry.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			IsDir:   entry.IsDir(),
			Path:    entryPath,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return files[i].Name < files[j].Name
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	tmpl := template.Must(template.New("dir").Funcs(template.FuncMap{
		"formatSize": formatSize,
		"formatTime": func(t time.Time) string { return t.Format("2006-01-02 15:04") },
	}).Parse(dirTemplate))

	data := struct {
		Path   string
		Files  []FileInfo
		Parent string
	}{
		Path:   reqPath,
		Files:  files,
		Parent: filepath.Dir(strings.TrimSuffix(reqPath, "/")),
	}

	tmpl.Execute(w, data)
}

func formatSize(size int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/GB)
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/MB)
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/KB)
	default:
		return fmt.Sprintf("%d B", size)
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	bytes      int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += int64(n)
	return n, err
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &responseWriter{ResponseWriter: w, statusCode: 200}
		start := time.Now()

		next.ServeHTTP(rw, r)

		record := state.AccessRecord{
			Time:       start,
			Path:       r.URL.Path,
			StatusCode: rw.statusCode,
			BytesSent:  rw.bytes,
			RemoteAddr: r.RemoteAddr,
		}

		
		state.UpdateAccessStats(record)
		// 已在 UpdateAccessStats 中保存
		

		logEntry := map[string]interface{}{
			"time":        start.Format(time.RFC3339),
			"path":        r.URL.Path,
			"method":      r.Method,
			"status":      rw.statusCode,
			"bytes":       rw.bytes,
			"remote_addr": r.RemoteAddr,
			"user_agent":  r.UserAgent(),
			"duration_ms": time.Since(start).Milliseconds(),
		}

		logData, _ := json.Marshal(logEntry)
		appendToAccessLog(string(logData))
	})
}

func appendToAccessLog(entry string) {
	logPath := config.GetAccessLogPath()
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	io.WriteString(f, entry+"\n")
}

const dirTemplate = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Index of {{.Path}}</title>
    <style>
        * { box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            margin: 0;
            padding: 20px;
            background: #f5f5f5;
        }
        .container {
            max-width: 900px;
            margin: 0 auto;
            background: white;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            overflow: hidden;
        }
        h1 {
            margin: 0;
            padding: 20px;
            background: #2563eb;
            color: white;
            font-size: 18px;
            font-weight: 500;
        }
        table {
            width: 100%;
            border-collapse: collapse;
        }
        th, td {
            padding: 12px 20px;
            text-align: left;
            border-bottom: 1px solid #eee;
        }
        th {
            background: #f9fafb;
            font-weight: 500;
            color: #6b7280;
            font-size: 12px;
            text-transform: uppercase;
        }
        tr:hover {
            background: #f9fafb;
        }
        a {
            color: #2563eb;
            text-decoration: none;
        }
        a:hover {
            text-decoration: underline;
        }
        .icon {
            margin-right: 8px;
        }
        .size, .time {
            color: #6b7280;
            font-size: 14px;
        }
        .back {
            padding: 15px 20px;
            border-bottom: 1px solid #eee;
        }
        @media (max-width: 600px) {
            .time { display: none; }
            th, td { padding: 10px 15px; }
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>📁 {{.Path}}</h1>
        {{if ne .Path "/"}}
        <div class="back">
            <a href="{{.Parent}}">⬆️ 返回上级目录</a>
        </div>
        {{end}}
        <table>
            <thead>
                <tr>
                    <th>名称</th>
                    <th>大小</th>
                    <th class="time">修改时间</th>
                </tr>
            </thead>
            <tbody>
                {{range .Files}}
                <tr>
                    <td>
                        <a href="{{.Path}}">
                            {{if .IsDir}}<span class="icon">📁</span>{{else}}<span class="icon">📄</span>{{end}}
                            {{.Name}}
                        </a>
                    </td>
                    <td class="size">{{if .IsDir}}-{{else}}{{formatSize .Size}}{{end}}</td>
                    <td class="time">{{formatTime .ModTime}}</td>
                </tr>
                {{end}}
                {{if not .Files}}
                <tr>
                    <td colspan="3" style="text-align: center; color: #6b7280; padding: 40px;">
                        📭 空目录
                    </td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
</body>
</html>`
