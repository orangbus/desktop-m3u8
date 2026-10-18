package core

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// CryptMethod AES 加密方式
type CryptMethod string

const (
	CryptMethodAES  CryptMethod = "AES-128"
	CryptMethodNONE CryptMethod = "NONE"
)

// Segment 一个 TS 分片
type Segment struct {
	URI      string  // 分片地址（可能是相对路径）
	KeyIndex int     // 对应的加密 Key 索引，-1 表示无加密
	Duration float64 // 分片时长
}

// Key AES 加密密钥信息
type Key struct {
	Method CryptMethod
	URI    string // key 地址（可能相对）
	IV     string // 十六进制 IV，可选
}

// Playlist 解析后的播放列表
type Playlist struct {
	Segments []*Segment
	Keys     map[int]*Key // KeyIndex -> Key
	EndList  bool
}

// ParseResult 解析结果
type ParseResult struct {
	URL      *url.URL
	Playlist *Playlist
	// KeysBytes keyIndex -> 已下载的 key 原始字节
	KeysBytes map[int][]byte
}

var linePattern = regexp.MustCompile(`([a-zA-Z-]+)=("[^"]+"|[^",]+)`)

// ParseM3U8 从文本解析播放列表（VOD / EVENT）。
func ParseM3U8(text string) (*Playlist, error) {
	lines := strings.Split(text, "\n")
	pl := &Playlist{Keys: make(map[int]*Key)}
	var seg *Segment
	keyIndex := -1
	extInf := false

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "#EXTM3U"):
			continue
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF"):
			// Master playlist：本版本取第一个子流，由调用方处理
			return nil, errMasterPlaylist
		case strings.HasPrefix(line, "#EXT-X-KEY"):
			params := parseLineParams(line)
			method := CryptMethod(params["METHOD"])
			if method == CryptMethodNONE {
				continue
			}
			keyIndex++
			k := &Key{Method: method, URI: params["URI"], IV: params["IV"]}
			pl.Keys[keyIndex] = k
		case strings.HasPrefix(line, "#EXTINF:"):
			extInf = true
			seg = &Segment{KeyIndex: keyIndex}
		case strings.HasPrefix(line, "#EXT-X-ENDLIST"):
			pl.EndList = true
		case strings.HasPrefix(line, "#"):
			// 其他标签忽略
			continue
		default:
			if !extInf || seg == nil {
				continue
			}
			seg.URI = line
			pl.Segments = append(pl.Segments, seg)
			seg = nil
			extInf = false
		}
	}
	if len(pl.Segments) == 0 {
		return nil, errors.New("no segment found in m3u8")
	}
	return pl, nil
}

var errMasterPlaylist = errors.New("master playlist")

// ParseMasterURI 当 m3u8 是 Master（多码率）时，返回第一个子流地址。
// 返回 (子流地址, true) 表示是 master；返回 ("", false) 表示不是 master。
func ParseMasterURI(text string) (string, bool) {
	lines := strings.Split(text, "\n")
	var pending bool
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF") {
			pending = true
			continue
		}
		if pending {
			if line != "" && !strings.HasPrefix(line, "#") {
				return line, true
			}
			pending = false
		}
	}
	return "", false
}

// ResolveURL 将相对地址解析为绝对地址。
func ResolveURL(base *url.URL, ref string) string {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	if strings.HasPrefix(ref, "//") {
		return base.Scheme + ":" + ref
	}
	if strings.HasPrefix(ref, "/") {
		return base.Scheme + "://" + base.Host + ref
	}
	u := *base
	u.Path = path.Join(path.Dir(u.Path), ref)
	u.RawQuery = ""
	return u.String()
}

// extractToken 从 m3u8 URL 提取标记值（index.m3u8 的父目录名）。
// 例：https://2609.v155p.com/20261011/5prtI9u6/index.m3u8 => 5prtI9u6
// 解析失败或路径层级不足时返回空串（此时不启用分片过滤）。
func extractToken(m3u8URL string) string {
	u, err := url.Parse(m3u8URL)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2]
}

func parseLineParams(line string) map[string]string {
	matches := linePattern.FindAllStringSubmatch(line, -1)
	params := make(map[string]string, len(matches))
	for _, m := range matches {
		params[m[1]] = strings.Trim(m[2], "\"")
	}
	return params
}

func parseIV(iv string) ([]byte, error) {
	if iv == "" {
		return nil, nil
	}
	s := strings.TrimPrefix(iv, "0x")
	return hexToBytes(s)
}

func hexToBytes(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("invalid hex length")
	}
	b := make([]byte, len(s)/2)
	for i := 0; i < len(b); i++ {
		v, err := strconv.ParseUint(s[i*2:i*2+2], 16, 8)
		if err != nil {
			return nil, err
		}
		b[i] = byte(v)
	}
	return b, nil
}
