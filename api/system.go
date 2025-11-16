package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"go-template/api/h"
	"go-template/utils/system"
)

const systemTag = "system-系统工具"

type versionResponse struct {
	Body struct {
		Name    string `json:"name" doc:"应用名称"`
		Version string `json:"version" doc:"版本号"`
		Channel string `json:"channel" doc:"更新频道"`
	} `json:"body"`
}

type openPathInput struct {
	Body struct {
		Path string `json:"path" doc:"目标路径" required:"true"`
	} `json:"body"`
}

type openEditorInput struct {
	Body struct {
		Path          string `json:"path" doc:"目标路径" required:"true"`
		Editor        string `json:"editor" doc:"目标编辑器(vscode/cursor/trae/zed/custom)" required:"true"`
		CustomCommand string `json:"customCommand,omitempty" doc:"自定义命令，使用 {{path}} 作为路径占位符"`
	} `json:"body"`
}

func registerSystemRoutes(group *huma.Group) {
	huma.Get(group, "/system/version", func(ctx context.Context, input *struct{}) (*versionResponse, error) {
		resp := &versionResponse{}
		resp.Body.Name = appInfo.Name
		resp.Body.Version = appInfo.Version
		resp.Body.Channel = appInfo.Channel
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-version"
		op.Summary = "获取应用版本信息"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/open-explorer", func(ctx context.Context, input *openPathInput) (*h.MessageResponse, error) {
		if err := system.OpenExplorer(input.Body.Path); err != nil {
			return nil, mapSystemError(err)
		}

		resp := h.NewMessageResponse("explorer opened")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-open-explorer"
		op.Summary = "打开文件管理器"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/open-terminal", func(ctx context.Context, input *openPathInput) (*h.MessageResponse, error) {
		if err := system.OpenTerminal(input.Body.Path); err != nil {
			return nil, mapSystemError(err)
		}

		resp := h.NewMessageResponse("terminal opened")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-open-terminal"
		op.Summary = "打开终端"
		op.Tags = []string{systemTag}
	})

	huma.Post(group, "/system/open-editor", func(ctx context.Context, input *openEditorInput) (*h.MessageResponse, error) {
		if err := system.OpenEditor(input.Body.Path, input.Body.Editor, input.Body.CustomCommand); err != nil {
			return nil, mapSystemError(err)
		}

		resp := h.NewMessageResponse("editor opened")
		resp.Status = http.StatusOK
		return resp, nil
	}, func(op *huma.Operation) {
		op.OperationID = "system-open-editor"
		op.Summary = "使用指定编辑器打开目录"
		op.Tags = []string{systemTag}
	})
}

func mapSystemError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, system.ErrUnsupportedOS):
		return huma.Error501NotImplemented(err.Error())
	case errors.Is(err, system.ErrNoFileManager),
		errors.Is(err, system.ErrNoTerminal):
		return huma.Error503ServiceUnavailable(err.Error())
	case errors.Is(err, system.ErrEditorCommandMissing):
		return huma.Error503ServiceUnavailable(err.Error())
	case errors.Is(err, system.ErrUnsupportedEditor),
		errors.Is(err, system.ErrCustomEditorCommand):
		return huma.Error400BadRequest(err.Error())
	default:
		return huma.Error500InternalServerError(err.Error())
	}
}
