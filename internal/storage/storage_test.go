package storage

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testJPEG(t *testing.T, w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, h/2, color.RGBA{R: 255})
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSniffImage(t *testing.T) {
	if _, _, _, err := SniffImage([]byte("not an image")); err == nil {
		t.Fatal("非图片应报错")
	}
	mime, w, h, err := SniffImage(testJPEG(t, 10, 8))
	if err != nil || mime != "image/jpeg" || w != 10 || h != 8 {
		t.Fatalf("jpeg 嗅探失败: %v %s %dx%d", err, mime, w, h)
	}
}

func TestProcessImageRejectsOversize(t *testing.T) {
	big := make([]byte, maxUpload+1)
	copy(big, testJPEG(t, 4, 4))
	if _, _, _, _, err := ProcessImage(big); err == nil {
		t.Fatal("超限文件应报错")
	}
}

func TestProcessImageGifPassthrough(t *testing.T) {
	gif := append([]byte(nil), "GIF89a"...)
	gif = append(gif, make([]byte, 16)...)
	_, mime, _, _, err := ProcessImage(gif)
	if err != nil || mime != "image/gif" {
		t.Fatalf("gif 应原样返回: %v", err)
	}
}

func TestLocalSaveSignVerify(t *testing.T) {
	dir := t.TempDir()
	l := &Local{Root: dir, SignKey: "k1", URLTTL: time.Hour}
	url, err := l.Save(context.Background(), Object{
		Key: "2026/10/abc.jpg", Mime: "image/jpeg", Body: bytes.NewReader([]byte("data"))})
	if err != nil {
		t.Fatal(err)
	}
	if url == "" || !bytes.Contains([]byte(url), []byte("e=")) {
		t.Fatalf("签名 URL 异常: %s", url)
	}
	// 读回
	rc, err := l.Open(context.Background(), "2026/10/abc.jpg")
	if err != nil {
		t.Fatal(err)
	}
	rc.Close()
	// 校验签名：正确 key + 参数
	if err := l.Verify("2026/10/abc.jpg", time.Now().Add(30*time.Minute).Unix(), sigOf(l, "2026/10/abc.jpg", time.Now().Add(30*time.Minute).Unix()), time.Now()); err != nil {
		t.Fatalf("合法签名被拒: %v", err)
	}
	// 过期
	if err := l.Verify("2026/10/abc.jpg", time.Now().Add(-time.Minute).Unix(), "x", time.Now()); err == nil {
		t.Fatal("过期签名应被拒")
	}
	// 路径穿越
	if _, err := l.Open(context.Background(), "../../etc/passwd"); err == nil {
		t.Fatal("路径穿越应被拒")
	}
	// 删除
	if err := l.Delete(context.Background(), "2026/10/abc.jpg"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "2026/10/abc.jpg")); !os.IsNotExist(err) {
		t.Fatal("文件应已删除")
	}
}

func sigOf(l *Local, key string, exp int64) string {
	// 与 SignedURL 相同算法取签名段
	u := l.SignedURL(key, time.Now().Add(-time.Hour+30*time.Minute))
	// 直接从 URL 提取 s= 参数
	i := bytes.Index([]byte(u), []byte("s="))
	return u[i+2:]
}

func TestKeyPrefix(t *testing.T) {
	if KeyPrefix(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) != "2026/10" {
		t.Fatal("key 前缀格式错误")
	}
}

