package api

import (
	"context"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"sub2api-ticket/internal/model"
	"sub2api-ticket/internal/storage"
)

type UploadHandler struct{ Deps }

// Upload POST /api/v1/uploads（multipart file 字段）
func (h *UploadHandler) Upload(c *gin.Context) {
	u := c.MustGet("user").(*model.User)
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 file 字段"})
		return
	}
	if fh.Size > 10<<20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件过大（上限 10MB）"})
		return
	}
	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件读取失败"})
		return
	}
	defer f.Close()
	data := make([]byte, fh.Size)
	// ReadFull：multipart Reader 单次 Read 可能短读，导致图片字节不完整
	if _, err := io.ReadFull(f, data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件读取失败"})
		return
	}
	svc := &UploadService{h.Deps}
	att, err := svc.SaveImage(context.Background(), u, data, fh.Filename)
	if err != nil {
		if isUserVisibleUploadErr(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			// 存储/数据库故障：不把内部细节透给前端
			c.JSON(http.StatusInternalServerError, gin.H{"error": "上传失败，请稍后再试"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"attachment": att})
}

// resolveAttachments 校验请求里的附件 URL 列表（Create/Reply 共用）
func resolveAttachments(h UploadHandler, c *gin.Context, u *model.User, urls []string) []storage.Attachment {
	svc := &UploadService{h.Deps}
	atts, err := svc.ResolveAttachments(u, urls, 5)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return nil
	}
	return atts
}
