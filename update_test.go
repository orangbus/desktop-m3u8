package main

import "testing"

func TestIsNewerVersion(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"1.0.3", "1.0.2", true},
		{"v1.1.0", "1.0.2", true},
		{"1.0.10", "1.0.9", true},  // 数字比较而非字典序
		{"2.0.0", "1.9.9", true},
		{"1.0.2", "1.0.2", false},  // 相同不提示
		{"1.0.1", "1.0.2", false},
		{"v1.0.2", "1.0.2", false},
		{"1.0.3-beta", "1.0.2", true}, // 忽略后缀仍按主版本号比较
		{"", "1.0.2", false},
	}
	for _, c := range cases {
		if got := isNewerVersion(c.latest, c.current); got != c.want {
			t.Errorf("isNewerVersion(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestPickAsset(t *testing.T) {
	assets := []ghAsset{
		{Name: "checksums.txt"},
		{Name: "desk-m3u8-windows-amd64.zip"},
		{Name: "desk-m3u8-darwin-arm64.zip"},
		{Name: "desk-m3u8-darwin-amd64.zip"},
		{Name: "desk-m3u8-linux-amd64.tar.gz"},
	}
	cases := []struct {
		os, arch, want string
	}{
		{"windows", "amd64", "desk-m3u8-windows-amd64.zip"},
		{"darwin", "arm64", "desk-m3u8-darwin-arm64.zip"},
		{"darwin", "amd64", "desk-m3u8-darwin-amd64.zip"},
		{"linux", "amd64", "desk-m3u8-linux-amd64.tar.gz"},
		// 平台无 arm64 产物时回退 amd64
		{"linux", "arm64", "desk-m3u8-linux-amd64.tar.gz"},
		{"windows", "arm64", "desk-m3u8-windows-amd64.zip"},
	}
	for _, c := range cases {
		got := pickAssetFor(assets, c.os, c.arch)
		if got == nil || got.Name != c.want {
			gotName := "<nil>"
			if got != nil {
				gotName = got.Name
			}
			t.Errorf("pickAsset(%s/%s) = %s, want %s", c.os, c.arch, gotName, c.want)
		}
	}
	// 没有匹配资源时返回 nil
	if got := pickAssetFor(nil, "windows", "amd64"); got != nil {
		t.Errorf("pickAsset(空资源) = %v, want nil", got)
	}
}
