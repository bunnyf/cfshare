package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestShareItemCreation(t *testing.T) {
	item := ShareItem{
		Path:      "/test/file.txt",
		Name:      "file.txt",
		ShareType: TypeFile,
		Size:      1024,
	}

	if item.Path != "/test/file.txt" {
		t.Errorf("unexpected path: %s", item.Path)
	}
	if item.Name != "file.txt" {
		t.Errorf("unexpected name: %s", item.Name)
	}
	if item.ShareType != TypeFile {
		t.Errorf("unexpected type: %s", item.ShareType)
	}
	if item.Size != 1024 {
		t.Errorf("unexpected size: %d", item.Size)
	}
}

func TestStateSingleItem(t *testing.T) {
	st := &State{
		ShareID: "test123",
		Mode:    ModeProtected,
		Items: []ShareItem{
			{Path: "/test/file.txt", Name: "file.txt", ShareType: TypeFile},
		},
		IsMulti: false,
	}

	if st.IsMulti {
		t.Error("single item should not be multi")
	}
	if len(st.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(st.Items))
	}
}

func TestStateMultiItems(t *testing.T) {
	st := &State{
		ShareID: "test123",
		Mode:    ModePublic,
		Items: []ShareItem{
			{Path: "/test/file1.txt", Name: "file1.txt", ShareType: TypeFile},
			{Path: "/test/file2.txt", Name: "file2.txt", ShareType: TypeFile},
			{Path: "/test/dir", Name: "dir", ShareType: TypeDir},
		},
		IsMulti: true,
	}

	if !st.IsMulti {
		t.Error("multiple items should be multi")
	}
	if len(st.Items) != 3 {
		t.Errorf("expected 3 items, got %d", len(st.Items))
	}
}

func TestFormatStatusSingleItem(t *testing.T) {
	st := &State{
		ShareID:   "test123",
		Mode:      ModeProtected,
		PublicURL: "https://share.example.com",
		Username:  "user",
		Password:  "pass",
		Items: []ShareItem{
			{Path: "/test/file.txt", Name: "file.txt", ShareType: TypeFile},
		},
		IsMulti: false,
	}

	output := st.FormatStatus()
	if !containsStr(output, "/test/file.txt") {
		t.Error("status should contain path")
	}
	if !containsStr(output, "file") {
		t.Error("status should contain type")
	}
}

func TestFormatStatusMultiItems(t *testing.T) {
	st := &State{
		ShareID:   "test123",
		Mode:      ModePublic,
		PublicURL: "https://share.example.com",
		Items: []ShareItem{
			{Path: "/test/file1.txt", Name: "file1.txt", ShareType: TypeFile},
			{Path: "/test/dir", Name: "dir", ShareType: TypeDir},
		},
		IsMulti: true,
	}

	output := st.FormatStatus()
	if !containsStr(output, "2 个项目") {
		t.Error("status should show item count")
	}
	if !containsStr(output, "file1.txt") {
		t.Error("status should contain file1.txt")
	}
	if !containsStr(output, "dir") {
		t.Error("status should contain dir")
	}
}

func TestFormatShareOutputSingleItem(t *testing.T) {
	st := &State{
		ShareID:   "test123",
		Mode:      ModeProtected,
		PublicURL: "https://share.example.com",
		Username:  "user",
		Password:  "pass",
		Items: []ShareItem{
			{Path: "/test/file.txt", Name: "file.txt", ShareType: TypeFile},
		},
		IsMulti: false,
	}

	output := st.FormatShareOutput()
	if !containsStr(output, "分享已启动") {
		t.Error("output should contain success message")
	}
	if !containsStr(output, "/test/file.txt") {
		t.Error("output should contain path")
	}
}

func TestFormatShareOutputMultiItems(t *testing.T) {
	st := &State{
		ShareID:   "test123",
		Mode:      ModePublic,
		PublicURL: "https://share.example.com",
		Items: []ShareItem{
			{Path: "/test/file1.txt", Name: "file1.txt", ShareType: TypeFile},
			{Path: "/test/file2.txt", Name: "file2.txt", ShareType: TypeFile},
		},
		IsMulti: true,
	}

	output := st.FormatShareOutput()
	if !containsStr(output, "2 个项目") {
		t.Error("output should show item count")
	}
	if !containsStr(output, "公开分享") {
		t.Error("output should warn about public share")
	}
}

func TestStateSaveLoadRoundTrip(t *testing.T) {
	// 使用临时目录
	tmpDir, err := os.MkdirTemp("", "cfshare-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 设置临时配置目录
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// 创建 .cfshare 目录
	cfshareDir := filepath.Join(tmpDir, ".cfshare")
	os.MkdirAll(cfshareDir, 0755)

	st := &State{
		ShareID:   "test123",
		Mode:      ModeProtected,
		PublicURL: "https://share.example.com",
		Username:  "user",
		Password:  "pass",
		Port:      8787,
		Items: []ShareItem{
			{Path: "/test/file1.txt", Name: "file1.txt", ShareType: TypeFile, Size: 100},
			{Path: "/test/dir", Name: "dir", ShareType: TypeDir, Size: 0},
		},
		IsMulti: true,
	}

	err = st.Save()
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.ShareID != st.ShareID {
		t.Errorf("ShareID mismatch: %s vs %s", loaded.ShareID, st.ShareID)
	}
	if loaded.IsMulti != st.IsMulti {
		t.Errorf("IsMulti mismatch: %v vs %v", loaded.IsMulti, st.IsMulti)
	}
	if len(loaded.Items) != len(st.Items) {
		t.Errorf("Items count mismatch: %d vs %d", len(loaded.Items), len(st.Items))
	}
}

func TestLoadLegacyFormat(t *testing.T) {
	// 使用临时目录
	tmpDir, err := os.MkdirTemp("", "cfshare-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 设置临时配置目录
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// 创建 .cfshare 目录
	cfshareDir := filepath.Join(tmpDir, ".cfshare")
	os.MkdirAll(cfshareDir, 0755)

	// 写入旧格式状态文件
	legacyJSON := `{
		"share_id": "legacy123",
		"mode": "protected",
		"path": "/legacy/file.txt",
		"share_type": "file",
		"port": 8787,
		"public_url": "https://example.com"
	}`
	statePath := filepath.Join(cfshareDir, "state.json")
	os.WriteFile(statePath, []byte(legacyJSON), 0600)

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 应该自动转换为 Items
	if len(loaded.Items) != 1 {
		t.Errorf("expected 1 item from legacy, got %d", len(loaded.Items))
	}
	if loaded.Items[0].Path != "/legacy/file.txt" {
		t.Errorf("unexpected path: %s", loaded.Items[0].Path)
	}
	if loaded.Items[0].Name != "file.txt" {
		t.Errorf("unexpected name: %s", loaded.Items[0].Name)
	}
	if loaded.IsMulti {
		t.Error("legacy single file should not be multi")
	}
}

func TestFormatLiveStatusNil(t *testing.T) {
	var st *State
	output := st.FormatLiveStatus()
	if !containsStr(output, "当前无活动分享") {
		t.Error("nil state should show no active share message")
	}
}

func TestFormatLiveStatusSingleItem(t *testing.T) {
	st := &State{
		ShareID:   "test123",
		Mode:      ModeProtected,
		PublicURL: "https://share.example.com",
		Username:  "user",
		Password:  "pass",
		Port:      8787,
		StartTime: time.Now().Add(-1 * time.Hour),
		Items: []ShareItem{
			{Path: "/test/file.txt", Name: "file.txt", ShareType: TypeFile, Size: 1048576},
		},
		IsMulti: false,
	}

	output := st.FormatLiveStatus()
	if !containsStr(output, "实时") {
		t.Error("live status should contain '实时'")
	}
	if !containsStr(output, "运行时间") {
		t.Error("live status should contain uptime")
	}
	if !containsStr(output, "流量统计") {
		t.Error("live status should contain traffic stats")
	}
	if !containsStr(output, "当前速度") {
		t.Error("live status should contain speed")
	}
	if !containsStr(output, "1.00 MB") {
		t.Error("live status should show file size")
	}
}

func TestFormatLiveStatusMultiItems(t *testing.T) {
	st := &State{
		ShareID:   "test123",
		Mode:      ModePublic,
		PublicURL: "https://share.example.com",
		Port:      8787,
		StartTime: time.Now().Add(-30 * time.Minute),
		Items: []ShareItem{
			{Path: "/test/file1.txt", Name: "file1.txt", ShareType: TypeFile, Size: 2048},
			{Path: "/test/dir", Name: "dir", ShareType: TypeDir},
		},
		IsMulti: true,
	}

	output := st.FormatLiveStatus()
	if !containsStr(output, "2 个项目") {
		t.Error("live status should show item count")
	}
	if !containsStr(output, "file1.txt") {
		t.Error("live status should show file1.txt")
	}
	if !containsStr(output, "dir") {
		t.Error("live status should show dir")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{5 * time.Second, "5秒"},
		{65 * time.Second, "1分 5秒"},
		{3661 * time.Second, "1时 1分 1秒"},
		{90061 * time.Second, "1天 1时 1分"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		b    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.00 KB"},
		{1048576, "1.00 MB"},
		{1073741824, "1.00 GB"},
	}

	for _, tt := range tests {
		got := formatBytes(tt.b)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.b, got, tt.want)
		}
	}
}

func TestCalculateSpeed(t *testing.T) {
	now := time.Now()

	// 近 30 秒内有请求
	records := []AccessRecord{
		{Time: now.Add(-10 * time.Second), BytesSent: 30000},
		{Time: now.Add(-5 * time.Second), BytesSent: 30000},
	}

	speed := calculateSpeed(records)
	if speed <= 0 {
		t.Error("speed should be > 0 for recent requests")
	}

	// 所有请求都超出 30 秒窗口
	oldRecords := []AccessRecord{
		{Time: now.Add(-60 * time.Second), BytesSent: 30000},
	}
	speed = calculateSpeed(oldRecords)
	if speed != 0 {
		t.Errorf("speed should be 0 for old requests, got %d", speed)
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
