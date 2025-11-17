package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"go.uber.org/zap"

	"code-kanban/api/h"
	"code-kanban/utils"
)

const uploadTag = "upload-上传"

type uploadController struct {
	cfg    *utils.AppConfig
	logger *zap.Logger
}

func registerUploadRoutes(group *huma.Group, cfg *utils.AppConfig, logger *zap.Logger) {
	ctrl := &uploadController{
		cfg:    cfg,
		logger: logger.Named("upload-controller"),
	}

	huma.Post(group, "/upload/clipboard-image", func(
		ctx context.Context,
		input *uploadClipboardImageInput,
	) (*h.ItemResponse[uploadImageResponse], error) {
		return ctrl.handleClipboardImage(ctx, input)
	}, func(op *huma.Operation) {
		op.OperationID = "upload-clipboard-image"
		op.Summary = "上传剪贴板图片"
		op.Tags = []string{uploadTag}
	})
}

func (c *uploadController) handleClipboardImage(
	ctx context.Context,
	input *uploadClipboardImageInput,
) (*h.ItemResponse[uploadImageResponse], error) {
	// 解码 base64 数据
	data, err := base64.StdEncoding.DecodeString(input.Body.Data)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid base64 data")
	}

	// 创建临时目录
	tempDir := filepath.Join(os.TempDir(), "aicode-kanban-clipboard")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, huma.Error500InternalServerError("failed to create temp directory", err)
	}

	// 生成文件名
	timestamp := time.Now().Format("20060102-150405")
	fileName := input.Body.FileName
	if fileName == "" {
		fileName = "pasted-image.png"
	}
	// 确保文件名唯一
	fileName = fmt.Sprintf("clipboard-%s-%s", timestamp, fileName)
	filePath := filepath.Join(tempDir, fileName)

	// 保存文件
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return nil, huma.Error500InternalServerError("failed to save image", err)
	}

	c.logger.Info("clipboard image saved",
		zap.String("path", filePath),
		zap.Int("size", len(data)))

	resp := h.NewItemResponse(uploadImageResponse{
		Path:     filePath,
		FileName: fileName,
		Size:     len(data),
	})
	resp.Status = http.StatusCreated
	return resp, nil
}

type uploadClipboardImageInput struct {
	Body struct {
		FileName string `json:"fileName" doc:"文件名"`
		Data     string `json:"data" doc:"图片数据（base64 编码）"`
	} `json:"body"`
}

type uploadImageResponse struct {
	Path     string `json:"path" doc:"文件路径"`
	FileName string `json:"fileName" doc:"文件名"`
	Size     int    `json:"size" doc:"文件大小（字节）"`
}
