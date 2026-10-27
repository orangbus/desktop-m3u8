package core

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"testing"
)

// 加密辅助：AES-128-CBC + PKCS7
func encryptCBC(plain, key, iv []byte) []byte {
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	data := append(append([]byte{}, plain...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	block, _ := aes.NewCipher(key)
	out := make([]byte, len(data))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, data)
	return out
}

// METHOD=NONE 之后，后续分片不得再关联之前的 key
func TestParseM3U8NoneResetsKeyIndex(t *testing.T) {
	text := `#EXTM3U
#EXT-X-TARGETDURATION:10
#EXT-X-KEY:METHOD=AES-128,URI="k1.key",IV=0x00000000000000000000000000000001
#EXTINF:10,
a.ts
#EXT-X-KEY:METHOD=NONE
#EXTINF:10,
b.ts
#EXT-X-ENDLIST`
	pl, err := ParseM3U8(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(pl.Segments) != 2 {
		t.Fatalf("segments = %d, want 2", len(pl.Segments))
	}
	if pl.Segments[0].KeyIndex != 0 {
		t.Fatalf("seg0 KeyIndex = %d, want 0", pl.Segments[0].KeyIndex)
	}
	if pl.Segments[1].KeyIndex != -1 {
		t.Fatalf("seg1 KeyIndex = %d, want -1（METHOD=NONE 后不得沿用旧 key）", pl.Segments[1].KeyIndex)
	}
}

// 无显式 IV 时，按 HLS 规范用媒体序列号作 IV
func TestKeyIVBytesFromSequence(t *testing.T) {
	iv, err := keyIVBytes("", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(iv) != 16 {
		t.Fatalf("iv len = %d, want 16", len(iv))
	}
	// 序列号 1 应在最后 8 字节的大端表示里
	if iv[15] != 1 || !bytes.Equal(iv[:8], make([]byte, 8)) {
		t.Fatalf("iv = %v, want leading zeros + seq 1", iv)
	}
	// MEDIA-SEQUENCE 起算：第 3 个分片（索引 2）序列号为 103
	text := "#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:101\n#EXTINF:1,\na.ts\n#EXTINF:1,\nb.ts\n#EXTINF:1,\nc.ts"
	pl, err := ParseM3U8(text)
	if err != nil {
		t.Fatal(err)
	}
	if pl.Segments[2].Seq != 103 {
		t.Fatalf("seg2 Seq = %d, want 103", pl.Segments[2].Seq)
	}
}

// 端到端：无 IV 的加密分片应能正确解密（过去用全 0 IV 会解出乱码）
func TestAesDecryptWithoutExplicitIV(t *testing.T) {
	key := bytes.Repeat([]byte{0x2b}, 16)
	iv, _ := keyIVBytes("", 5)
	secret := []byte("hello hls segment payload!!")
	enc := encryptCBC(secret, key, iv)
	got, err := aesDecrypt(enc, key, iv)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatalf("decrypt mismatch: got %q", got)
	}
}

func TestSanitizeFilenameReserved(t *testing.T) {
	if got := sanitizeFilename("CON"); got != "_CON" {
		t.Fatalf("CON => %q, want _CON", got)
	}
	if got := sanitizeFilename("a<b>:c"); got != "abc" {
		t.Fatalf("a<b>:c => %q, want abc", got)
	}
}

func TestUniqueOutputPath(t *testing.T) {
	dir := t.TempDir()
	p1 := uniqueOutputPath(dir, "v.mp4")
	if p1 == "" {
		t.Fatal("first path empty")
	}
	writeOK := func(p string) {
		t.Helper()
		if err := writeFileForTest(p); err != nil {
			t.Fatal(err)
		}
	}
	writeOK(p1)
	p2 := uniqueOutputPath(dir, "v.mp4")
	if p2 == p1 {
		t.Fatal("second path must differ when first exists")
	}
}
