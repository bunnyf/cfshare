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
	sharePath string
	shareType state.ShareType
	state     *state.State
	stateMu   sync.Mutex
	srv       *http.Server
}

func NewServer(sharePath string, st *state.State) (*Server, error) {
	absPath, err := filepath.Abs(sharePath)
	if err != nil {
		return nil, fmt.Errorf("get absolute path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("stat path: %w", err)
	}

	shareType := state.TypeFile
	if info.IsDir() {
		shareType = state.TypeDir
	}

	st.Path = absPath
	st.ShareType = shareType

	return &Server{
		sharePath: absPath,
		shareType: shareType,
		state:     st,
	}, nil
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
	w.Header().Set("Cache-Control", "no-store")

	if s.shareType == state.TypeFile {
		s.serveFile(w, r)
	} else {
		s.serveDir(w, r)
	}
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request) {
	reqPath := r.URL.Path
	fileName := filepath.Base(s.sharePath)

	if reqPath != "/" && reqPath != "/"+fileName {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	http.ServeFile(w, r, s.sharePath)
}

func (s *Server) serveDir(w http.ResponseWriter, r *http.Request) {
	reqPath := filepath.Clean(r.URL.Path)
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
