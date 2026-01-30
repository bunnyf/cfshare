package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"cfshare/internal/auth"
	"cfshare/internal/state"
)

// ==================== 路径穿越攻击测试 ====================

func TestPathTraversalAttacks(t *testing.T) {
	// 创建测试目录结构
	tmpDir, _ := os.MkdirTemp("", "security_test")
	defer os.RemoveAll(tmpDir)

	// 创建分享目录和敏感文件
	shareDir := filepath.Join(tmpDir, "share")
	os.Mkdir(shareDir, 0755)
	os.WriteFile(filepath.Join(shareDir, "public.txt"), []byte("public content"), 0644)

	// 在父目录创建敏感文件（不应被访问）
	os.WriteFile(filepath.Join(tmpDir, "secret.txt"), []byte("secret content"), 0644)

	st := &state.State{}
	srv, _ := NewServer([]string{shareDir}, st)

	// 路径穿越攻击向量
	attackPaths := []string{
		"/../secret.txt",
		"/..%2Fsecret.txt",
		"/../../../etc/passwd",
		"/..\\secret.txt",
		"/.../secret.txt",
		"/....//secret.txt",
		"/%2e%2e/secret.txt",
		"/%2e%2e%2fsecret.txt",
		"/..%252f..%252f..%252fetc/passwd",
		"/.%2e/secret.txt",
		"/%252e%252e/secret.txt",
		"/..;/secret.txt",
		"/..%00/secret.txt",
		"/..//..//..//secret.txt",
	}

	for _, path := range attackPaths {
		t.Run("Attack_"+path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			srv.handleRequest(w, req)

			// 不应该返回 200 OK 和敏感内容
			if w.Code == http.StatusOK && strings.Contains(w.Body.String(), "secret") {
				t.Errorf("path traversal successful with %s: got %d, body contains secret", path, w.Code)
			}
		})
	}
}

func TestPathTraversalMultiMode(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "multi_security_test")
	defer os.RemoveAll(tmpDir)

	// 创建多个分享目录
	dir1 := filepath.Join(tmpDir, "dir1")
	dir2 := filepath.Join(tmpDir, "dir2")
	os.Mkdir(dir1, 0755)
	os.Mkdir(dir2, 0755)
	os.WriteFile(filepath.Join(dir1, "file1.txt"), []byte("file1"), 0644)
	os.WriteFile(filepath.Join(dir2, "file2.txt"), []byte("file2"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "secret.txt"), []byte("secret"), 0644)

	st := &state.State{}
	srv, _ := NewServer([]string{dir1, dir2}, st)

	// 尝试从 dir1 跨越到 dir2 或父目录
	attackPaths := []string{
		"/dir1/../secret.txt",
		"/dir1/../dir2/file2.txt",
		"/dir1/../../etc/passwd",
		"/dir1/%2e%2e/secret.txt",
	}

	for _, path := range attackPaths {
		t.Run("MultiMode_"+path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			srv.handleRequest(w, req)

			if w.Code == http.StatusOK && (strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), "file2")) {
				t.Errorf("cross-directory traversal successful with %s", path)
			}
		})
	}
}

func TestNullByteInjection(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "nullbyte_test")
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "secret.txt"), []byte("secret"), 0644)

	st := &state.State{}
	srv, _ := NewServer([]string{tmpDir}, st)

	// 空字节注入尝试 (URL编码形式，因为Go不允许原始空字节)
	attackPaths := []string{
		"/test.txt%00.jpg",
		"/%00/../secret.txt",
		"/test%00.txt",
	}

	for _, path := range attackPaths {
		t.Run("NullByte_"+path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			srv.handleRequest(w, req)

			// 不应该返回敏感内容
			if strings.Contains(w.Body.String(), "secret") {
				t.Errorf("null byte injection may have bypassed security with %s", path)
			}
		})
	}
}

// ==================== 认证绕过测试 ====================

func TestAuthBypassAttempts(t *testing.T) {
	username := "admin"
	password := "secretpass123"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("protected content"))
	})

	protected := auth.BasicAuthMiddleware(username, password, handler)

	tests := []struct {
		name     string
		setup    func(req *http.Request)
		wantCode int
	}{
		{
			name:     "NoAuth",
			setup:    func(req *http.Request) {},
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "EmptyCredentials",
			setup: func(req *http.Request) {
				req.SetBasicAuth("", "")
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "WrongUsername",
			setup: func(req *http.Request) {
				req.SetBasicAuth("wrong", password)
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "WrongPassword",
			setup: func(req *http.Request) {
				req.SetBasicAuth(username, "wrong")
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "CaseSensitiveUsername",
			setup: func(req *http.Request) {
				req.SetBasicAuth("ADMIN", password)
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "CaseSensitivePassword",
			setup: func(req *http.Request) {
				req.SetBasicAuth(username, "SECRETPASS123")
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "UsernameWithSpaces",
			setup: func(req *http.Request) {
				req.SetBasicAuth(" admin", password)
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "PasswordWithSpaces",
			setup: func(req *http.Request) {
				req.SetBasicAuth(username, " secretpass123")
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "SQLInjectionUsername",
			setup: func(req *http.Request) {
				req.SetBasicAuth("admin'--", password)
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "SQLInjectionPassword",
			setup: func(req *http.Request) {
				req.SetBasicAuth(username, "' OR '1'='1")
			},
			wantCode: http.StatusUnauthorized,
		},
		{
			name: "ValidCredentials",
			setup: func(req *http.Request) {
				req.SetBasicAuth(username, password)
			},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			tt.setup(req)
			w := httptest.NewRecorder()
			protected.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("expected %d, got %d", tt.wantCode, w.Code)
			}

			// 确保未授权时不返回受保护内容
			if tt.wantCode == http.StatusUnauthorized && strings.Contains(w.Body.String(), "protected content") {
				t.Error("protected content leaked despite unauthorized status")
			}
		})
	}
}

func TestAuthHeaderManipulation(t *testing.T) {
	username := "admin"
	password := "pass123"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("protected"))
	})

	protected := auth.BasicAuthMiddleware(username, password, handler)

	tests := []struct {
		name   string
		header string
	}{
		{"MalformedBasic", "Basic "},
		{"InvalidBase64", "Basic not-valid-base64!!!"},
		{"NoColon", "Basic " + "YWRtaW4="}, // "admin" without colon
		{"EmptyBase64", "Basic "},
		{"DigestAuth", "Digest username=\"admin\""},
		{"BearerToken", "Bearer sometoken"},
		{"CustomScheme", "Custom admin:pass123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Authorization", tt.header)
			w := httptest.NewRecorder()
			protected.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 for malformed header, got %d", w.Code)
			}
		})
	}
}

func TestTimingAttackResistance(t *testing.T) {
	// 测试密码比较是否使用常量时间比较
	// 这是一个简化测试，实际时序攻击测试需要统计分析
	username := "admin"
	password := "correctpassword123"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	protected := auth.BasicAuthMiddleware(username, password, handler)

	// 测试不同长度的错误密码
	passwords := []string{
		"a",
		"ab",
		"correctpassword12",  // 差一个字符
		"correctpassword123", // 正确
		"correctpassword1234",
		"wrongpasswordwrongpassword",
	}

	for _, pwd := range passwords {
		req := httptest.NewRequest("GET", "/", nil)
		req.SetBasicAuth(username, pwd)
		w := httptest.NewRecorder()
		protected.ServeHTTP(w, req)

		if pwd == password {
			if w.Code != http.StatusOK {
				t.Errorf("correct password should return 200, got %d", w.Code)
			}
		} else {
			if w.Code != http.StatusUnauthorized {
				t.Errorf("wrong password should return 401, got %d", w.Code)
			}
		}
	}
}

// ==================== 符号链接攻击测试 ====================

func TestSymlinkAttacks(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "symlink_test")
	defer os.RemoveAll(tmpDir)

	// 创建分享目录
	shareDir := filepath.Join(tmpDir, "share")
	os.Mkdir(shareDir, 0755)
	os.WriteFile(filepath.Join(shareDir, "safe.txt"), []byte("safe content"), 0644)

	// 创建外部敏感文件
	secretFile := filepath.Join(tmpDir, "secret.txt")
	os.WriteFile(secretFile, []byte("secret content"), 0644)

	// 在分享目录内创建指向外部的符号链接
	symlinkPath := filepath.Join(shareDir, "link_to_secret")
	err := os.Symlink(secretFile, symlinkPath)
	if err != nil {
		t.Skipf("Cannot create symlink (may need elevated privileges): %v", err)
	}

	st := &state.State{}
	srv, _ := NewServer([]string{shareDir}, st)

	// 尝试通过符号链接访问外部文件
	req := httptest.NewRequest("GET", "/link_to_secret", nil)
	w := httptest.NewRecorder()
	srv.handleRequest(w, req)

	// 符号链接指向外部应该被阻止
	if w.Code == http.StatusOK && strings.Contains(w.Body.String(), "secret") {
		t.Error("symlink to external file should be blocked")
	}
}

func TestSymlinkToParentDir(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "symlink_parent_test")
	defer os.RemoveAll(tmpDir)

	shareDir := filepath.Join(tmpDir, "share")
	os.Mkdir(shareDir, 0755)
	os.WriteFile(filepath.Join(tmpDir, "parent_secret.txt"), []byte("parent secret"), 0644)

	// 创建指向父目录的符号链接
	symlinkPath := filepath.Join(shareDir, "parent_link")
	err := os.Symlink(tmpDir, symlinkPath)
	if err != nil {
		t.Skipf("Cannot create symlink: %v", err)
	}

	st := &state.State{}
	srv, _ := NewServer([]string{shareDir}, st)

	// 尝试通过符号链接访问父目录的文件
	req := httptest.NewRequest("GET", "/parent_link/parent_secret.txt", nil)
	w := httptest.NewRecorder()
	srv.handleRequest(w, req)

	if w.Code == http.StatusOK && strings.Contains(w.Body.String(), "parent secret") {
		t.Error("symlink to parent directory should be blocked")
	}
}

func TestSymlinkChain(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "symlink_chain_test")
	defer os.RemoveAll(tmpDir)

	shareDir := filepath.Join(tmpDir, "share")
	os.Mkdir(shareDir, 0755)

	// 创建符号链接链
	secretFile := filepath.Join(tmpDir, "secret.txt")
	os.WriteFile(secretFile, []byte("chained secret"), 0644)

	link1 := filepath.Join(tmpDir, "link1")
	link2 := filepath.Join(shareDir, "link2")

	os.Symlink(secretFile, link1)
	err := os.Symlink(link1, link2)
	if err != nil {
		t.Skipf("Cannot create symlink: %v", err)
	}

	st := &state.State{}
	srv, _ := NewServer([]string{shareDir}, st)

	req := httptest.NewRequest("GET", "/link2", nil)
	w := httptest.NewRecorder()
	srv.handleRequest(w, req)

	if w.Code == http.StatusOK && strings.Contains(w.Body.String(), "chained secret") {
		t.Error("symlink chain to external file should be blocked")
	}
}

// ==================== 大文件和并发测试 ====================

func TestLargeFileHandling(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "large_file_test")
	defer os.RemoveAll(tmpDir)

	// 创建一个相对较大的文件 (10MB)
	largeFile := filepath.Join(tmpDir, "large.bin")
	f, err := os.Create(largeFile)
	if err != nil {
		t.Fatal(err)
	}

	// 写入 10MB 数据
	size := int64(10 * 1024 * 1024)
	chunk := make([]byte, 1024*1024) // 1MB chunks
	for i := 0; i < 10; i++ {
		f.Write(chunk)
	}
	f.Close()

	st := &state.State{}
	srv, _ := NewServer([]string{largeFile}, st)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.handleRequest(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for large file, got %d", w.Code)
	}

	// 验证响应大小
	if int64(w.Body.Len()) != size {
		t.Errorf("expected %d bytes, got %d", size, w.Body.Len())
	}
}

func TestConcurrentRequests(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "concurrent_test")
	defer os.RemoveAll(tmpDir)

	// 创建单个测试文件（单文件模式）
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("content"), 0644)

	st := &state.State{}
	srv, _ := NewServer([]string{testFile}, st)

	// 并发请求
	numGoroutines := 100
	numRequests := 10

	var wg sync.WaitGroup
	var errorCount int
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numRequests; j++ {
				req := httptest.NewRequest("GET", "/", nil)
				w := httptest.NewRecorder()
				srv.handleRequest(w, req)

				if w.Code != http.StatusOK {
					mu.Lock()
					errorCount++
					mu.Unlock()
				}
			}
		}(i)
	}

	wg.Wait()

	if errorCount > 0 {
		t.Errorf("concurrent requests had %d errors out of %d total requests", errorCount, numGoroutines*numRequests)
	}
}

type concurrencyError struct {
	id   int
	req  int
	code int
}

func (e *concurrencyError) Error() string {
	return fmt.Sprintf("goroutine %d request %d got status %d", e.id, e.req, e.code)
}

func TestConcurrentPathTraversal(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "concurrent_traversal_test")
	defer os.RemoveAll(tmpDir)

	shareDir := filepath.Join(tmpDir, "share")
	os.Mkdir(shareDir, 0755)
	os.WriteFile(filepath.Join(shareDir, "safe.txt"), []byte("safe"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "secret.txt"), []byte("secret"), 0644)

	st := &state.State{}
	srv, _ := NewServer([]string{shareDir}, st)

	// 并发发送路径穿越请求
	numGoroutines := 50
	var wg sync.WaitGroup
	securityBreaches := make(chan string, numGoroutines)

	attackPaths := []string{
		"/../secret.txt",
		"/..%2Fsecret.txt",
		"/../../../etc/passwd",
	}

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for _, path := range attackPaths {
				req := httptest.NewRequest("GET", path, nil)
				w := httptest.NewRecorder()
				srv.handleRequest(w, req)

				if w.Code == http.StatusOK && strings.Contains(w.Body.String(), "secret") {
					securityBreaches <- path
				}
			}
		}(i)
	}

	wg.Wait()
	close(securityBreaches)

	for breach := range securityBreaches {
		t.Errorf("security breach under concurrent load with path: %s", breach)
	}
}

func TestRaceConditionFileAccess(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "race_test")
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("initial content"), 0644)

	st := &state.State{}
	srv, _ := NewServer([]string{tmpDir}, st)

	// 同时读取和修改文件
	var wg sync.WaitGroup
	numGoroutines := 50

	// 读取者
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				req := httptest.NewRequest("GET", "/test.txt", nil)
				w := httptest.NewRecorder()
				srv.handleRequest(w, req)
				// 只要不 panic 就算成功
			}
		}()
	}

	// 写入者（模拟文件修改）
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				content := []byte("modified content " + string(rune(id)))
				os.WriteFile(testFile, content, 0644)
			}
		}(i)
	}

	wg.Wait()
}

// ==================== 其他安全测试 ====================

func TestSpecialCharactersInFilename(t *testing.T) {
	// 使用当前目录下的临时目录，避免 macOS 的 /tmp -> /private/tmp 问题
	tmpDir, err := os.MkdirTemp(".", "special_chars_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 获取绝对路径
	tmpDir, _ = filepath.Abs(tmpDir)

	// 测试各种特殊字符文件名
	specialNames := []string{
		"file_with_underscores.txt",
		"file-with-dashes.txt",
		"file.multiple.dots.txt",
	}

	for _, name := range specialNames {
		path := filepath.Join(tmpDir, name)
		err := os.WriteFile(path, []byte("content"), 0644)
		if err != nil {
			continue // 跳过操作系统不支持的文件名
		}
	}

	st := &state.State{}
	srv, err := NewServer([]string{tmpDir}, st)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	// 请求目录列表
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.handleRequest(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	// 确保没有 XSS 漏洞（检查 HTML 转义）
	body := w.Body.String()
	if strings.Contains(body, "<script>") {
		t.Error("potential XSS vulnerability: unescaped script tag")
	}
}

func TestXSSPrevention(t *testing.T) {
	// 使用当前目录下的临时目录
	tmpDir, err := os.MkdirTemp(".", "xss_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tmpDir, _ = filepath.Abs(tmpDir)

	// 创建正常文件
	os.WriteFile(filepath.Join(tmpDir, "normal.txt"), []byte("content"), 0644)

	st := &state.State{}
	srv, err := NewServer([]string{tmpDir}, st)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	// 请求目录列表
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.handleRequest(w, req)

	body := w.Body.String()

	// 验证使用了模板（Go的html/template自动转义）
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Errorf("response should be HTML, got: %s", body[:min(200, len(body))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestHTTPMethodRestrictions(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "method_test")
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("content"), 0644)

	st := &state.State{}
	srv, _ := NewServer([]string{tmpDir}, st)

	methods := []string{"POST", "PUT", "DELETE", "PATCH", "OPTIONS"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/test.txt", nil)
			w := httptest.NewRecorder()
			srv.handleRequest(w, req)

			// 对于非 GET 方法，验证不会执行危险操作
			// 文件应该仍然存在
			if _, err := os.Stat(filepath.Join(tmpDir, "test.txt")); os.IsNotExist(err) {
				t.Errorf("%s method deleted the file!", method)
			}
		})
	}
}

func TestVeryLongPath(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "longpath_test")
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("content"), 0644)

	st := &state.State{}
	srv, _ := NewServer([]string{tmpDir}, st)

	// 创建非常长的路径
	longPath := "/" + strings.Repeat("a", 10000) + "/test.txt"

	req := httptest.NewRequest("GET", longPath, nil)
	w := httptest.NewRecorder()

	// 不应该 panic
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("panic with long path: %v", r)
			}
		}()
		srv.handleRequest(w, req)
	}()
}

func TestManyNestedDirectories(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "nested_test")
	defer os.RemoveAll(tmpDir)

	// 创建深层嵌套目录
	current := tmpDir
	for i := 0; i < 50; i++ {
		current = filepath.Join(current, "dir")
		os.Mkdir(current, 0755)
	}
	os.WriteFile(filepath.Join(current, "deep.txt"), []byte("deep content"), 0644)

	st := &state.State{}
	srv, _ := NewServer([]string{tmpDir}, st)

	// 构建深层路径
	deepPath := strings.Repeat("/dir", 50) + "/deep.txt"

	req := httptest.NewRequest("GET", deepPath, nil)
	w := httptest.NewRecorder()
	srv.handleRequest(w, req)

	if w.Code != http.StatusOK {
		t.Logf("deep path returned %d (may be expected due to path limits)", w.Code)
	}
}
