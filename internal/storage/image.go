package storage

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"

	"golang.org/x/image/draw"
	_ "image/gif"
	xwebp "golang.org/x/image/webp"
)

// 最大边长与压缩质量
const (
	maxDimension = 1920
	jpegQuality  = 82
	maxUpload    = 10 << 20 // 10MB
)

// allowedMime magic bytes 校验通过后允许的类型
var allowedMime = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
}

// SniffImage 校验上传字节：magic bytes 判型（不信任扩展名与 Header），
// 返回真实 mime、尺寸。GIF 尺寸由 header 解析。
func SniffImage(data []byte) (mime string, w, h int, err error) {
	switch {
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		mime = "image/jpeg"
	case len(data) >= 8 && string(data[0:8]) == "\x89PNG\r\n\x1a\n":
		mime = "image/png"
	case len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		mime = "image/webp"
	case len(data) >= 6 && (string(data[0:6]) == "GIF87a" || string(data[0:6]) == "GIF89a"):
		mime = "image/gif"
	default:
		return "", 0, 0, fmt.Errorf("不支持的图片格式（仅 jpg/png/webp/gif）")
	}
	switch mime {
	case "image/webp":
		cfg, e := xwebp.DecodeConfig(bytes.NewReader(data))
		if e != nil {
			return "", 0, 0, fmt.Errorf("webp 解析失败: %w", e)
		}
		return mime, cfg.Width, cfg.Height, nil
	}
	cfg, _, e := image.DecodeConfig(bytes.NewReader(data))
	if e != nil {
		return "", 0, 0, fmt.Errorf("图片解析失败: %w", e)
	}
	return mime, cfg.Width, cfg.Height, nil
}

// ProcessImage 校验 + 压缩：gif/webp 原样保留（webp 已足够小、gif 保动图），
// jpeg/png 长边 >1920 时缩放，随后按质量 82 重编码。
// 输入为完整字节（上传文件 ≤10MB，内存可承受）。
func ProcessImage(data []byte) (out []byte, mime string, w, h int, err error) {
	if len(data) > maxUpload {
		return nil, "", 0, 0, fmt.Errorf("文件过大（上限 10MB）")
	}
	mime, w, h, err = SniffImage(data)
	if err != nil {
		return
	}
	if !allowedMime[mime] {
		return nil, "", 0, 0, fmt.Errorf("不支持的图片格式")
	}
	if mime == "image/gif" || mime == "image/webp" {
		return data, mime, w, h, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", 0, 0, fmt.Errorf("图片解码失败: %w", err)
	}
	b := img.Bounds()
	if b.Dx() > maxDimension || b.Dy() > maxDimension {
		img = scaleDown(img, maxDimension)
		b = img.Bounds()
	}
	var buf bytes.Buffer
	switch mime {
	case "image/jpeg":
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality})
	case "image/png":
		// png 无损压缩参数标准库有限，缩放本身已显著减小体积
		err = png.Encode(&buf, img)
	}
	if err != nil {
		return nil, "", 0, 0, fmt.Errorf("重编码失败: %w", err)
	}
	if buf.Len() >= len(data) {
		// 重编码反而更大（常见于小图），保留原图
		return data, mime, w, h, nil
	}
	return buf.Bytes(), mime, b.Dx(), b.Dy(), nil
}

// scaleDown 等比缩放至长边 ≤ max
func scaleDown(img image.Image, max int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= max && h <= max {
		return img
	}
	var nw, nh int
	if w >= h {
		nw, nh = max, h*max/w
	} else {
		nw, nh = w*max/h, max
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}

var _ io.Reader = (*bytes.Reader)(nil)
