package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"cfshare/internal/state"
)

func TestNewServerSingleFile(t *testing.T) {
	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "test*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString("test content")
	tmpFile.Close()

	st := &state.State{}
	srv, err := NewServer([]string{tmpFile.Name()}, st)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if srv.isMulti {
		t.Error("single file should not be multi mode")
	}
	if len(srv.items) != 1 {
		t.Errorf("expected 1 item, got %d", len(srv.items))
	}
	if srv.items[0].ShareType != state.TypeFile {
		t.Errorf("expected TypeFile, got %s", srv.items[0].ShareType)
	}
}

func TestNewServerSingleDir(t *testing.T) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "testdir")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	st := &state.State{}
	srv, err := NewServer([]string{tmpDir}, st)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if srv.isMulti {
		t.Error("single dir should not be multi mode")
	}
	if srv.items[0].ShareType != state.TypeDir {
		t.Errorf("expected TypeDir, got %s", srv.items[0].ShareType)
	}
}

func TestNewServerMultiItems(t *testing.T) {
	// 创建临时文件和目录
	tmpFile, _ := os.CreateTemp("", "test*.txt")
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString("test")
	tmpFile.Close()

	tmpDir, _ := os.MkdirTemp("", "testdir")
	defer os.RemoveAll(tmpDir)

	st := &state.State{}
	srv, err := NewServer([]string{tmpFile.Name(), tmpDir}, st)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	if !srv.isMulti {
		t.Error("multiple items should be multi mode")
	}
	if len(srv.items) != 2 {
		t.Errorf("expected 2 items, got %d", len(srv.items))
	}
}

func TestNewServerNameConflict(t *testing.T) {
	// 创建两个同名文件在不同目录
	tmpDir1, _ := os.MkdirTemp("", "dir1")
	tmpDir2, _ := os.MkdirTemp("", "dir2")
	defer os.RemoveAll(tmpDir1)
	defer os.RemoveAll(tmpDir2)

	file1 := filepath.Join(tmpDir1, "test.txt")
	file2 := filepath.Join(tmpDir2, "test.txt")
	os.WriteFile(file1, []byte("content1"), 0644)
	os.WriteFile(file2, []byte("content2"), 0644)

	st := &state.State{}
	_, err := NewServer([]string{file1, file2}, st)
	if err == nil {
		t.Error("expected error for name conflict")
	}
}

func TestNewServerNoPath(t *testing.T) {
	st := &state.State{}
	_, err := NewServer([]string{}, st)
	if err == nil {
		t.Error("expected error for empty paths")
	}
}

func TestNewServerInvalidPath(t *testing.T) {
	st := &state.State{}
	_, err := NewServer([]string{"/nonexistent/path/file.txt"}, st)
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestHandleMultiShareRoot(t *testing.T) {
	// 创建测试文件
	tmpFile, _ := os.CreateTemp("", "file1*.txt")
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString("content")
	tmpFile.Close()

	tmpDir, _ := os.MkdirTemp("", "dir1")
	defer os.RemoveAll(tmpDir)

	st := &state.State{}
	srv, _ := NewServer([]string{tmpFile.Name(), tmpDir}, st)

	// 测试根路径
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.handleRequest(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !contains(body, filepath.Base(tmpFile.Name())) {
		t.Error("response should contain file name")
	}
	if !contains(body, filepath.Base(tmpDir)) {
		t.Error("response should contain dir name")
	}
}

func TestHandleMultiShareFile(t *testing.T) {
	// 创建测试文件
	tmpDir, _ := os.MkdirTemp("", "testdir")
	defer os.RemoveAll(tmpDir)

	file1 := filepath.Join(tmpDir, "file1.txt")
	os.WriteFile(file1, []byte("file1 content"), 0644)

	file2 := filepath.Join(tmpDir, "file2.txt")
	os.WriteFile(file2, []byte("file2 content"), 0644)

	st := &state.State{}
	srv, _ := NewServer([]string{file1, file2}, st)

	// 测试下载文件
	req := httptest.NewRequest("GET", "/file1.txt", nil)
	w := httptest.NewRecorder()
	srv.handleRequest(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	if w.Body.String() != "file1 content" {
		t.Errorf("unexpected content: %s", w.Body.String())
	}
}

func TestHandleMultiShareNotFound(t *testing.T) {
	tmpFile, _ := os.CreateTemp("", "test*.txt")
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString("content")
	tmpFile.Close()

	st := &state.State{}
	srv, _ := NewServer([]string{tmpFile.Name()}, st)
	srv.isMulti = true // 强制多文件模式测试

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.handleMultiShare(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestPathTraversalPrevention(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "testdir")
	defer os.RemoveAll(tmpDir)

	// 创建子目录和文件
	subDir := filepath.Join(tmpDir, "sub")
	os.Mkdir(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "secret.txt"), []byte("secret"), 0644)

	st := &state.State{}
	srv, _ := NewServer([]string{subDir}, st)
	srv.isMulti = true
	srv.itemMap = map[string]*state.ShareItem{
		"sub": &srv.items[0],
	}

	// 尝试路径遍历
	req := httptest.NewRequest("GET", "/sub/../../../etc/passwd", nil)
	w := httptest.NewRecorder()
	srv.handleMultiShare(w, req)

	// 应该返回 403 或 404，不应该返回 200
	if w.Code == http.StatusOK {
		t.Error("path traversal should be blocked")
	}
}

func TestSingleFileModeBackwardCompatibility(t *testing.T) {
	tmpFile, _ := os.CreateTemp("", "test*.txt")
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString("test content")
	tmpFile.Close()

	st := &state.State{}
	srv, _ := NewServer([]string{tmpFile.Name()}, st)

	// 单文件模式应该直接返回文件
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.handleRequest(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	if w.Body.String() != "test content" {
		t.Errorf("unexpected content: %s", w.Body.String())
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
