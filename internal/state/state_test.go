package state

import (
	"os"
	"path/filepath"
	"testing"
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

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
