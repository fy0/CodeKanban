package api

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"testing"

	"code-kanban/api/h"
	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/service/websession"
	"code-kanban/utils"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

func TestWebSessionCommandGroupDetailDecodesToolIDs(t *testing.T) {
	model.DBClose()
	if err := model.InitWithDSN(filepath.Join(t.TempDir(), "command-groups.db"), 0, true); err != nil {
		t.Fatalf("InitWithDSN: %v", err)
	}
	t.Cleanup(model.DBClose)

	session := tables.WebSessionTable{
		ProjectID: "project-tools", Agent: "devin", Title: "Tool details",
		Cwd: t.TempDir(), Status: "done",
	}
	session.Init()
	if err := model.GetDB().Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	manager, err := websession.NewManager(websession.Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	app := fiber.New(fiber.Config{Immutable: true})
	_, group := h.NewAPI(app, &utils.AppConfig{})
	registerWebSessionRoutes(app, group, manager, zap.NewNop())
	basePath := "/api/v1/projects/" + session.ProjectID + "/web-sessions/" + session.ID + "/command-groups/"

	for index, toolID := range []string{
		"call-1",
		"exec:0#424df1d20cac4c3ca8f9526de184c1ce",
		"read:1#8fab259ae80c4e1b86b0261673902532",
		"tool/with/slashes",
		"tool+with spaces?query=value&other=1",
		"tool%23literal%2Fencoded",
		"tool%invalid",
		"工具:读取#1",
	} {
		t.Run(toolID, func(t *testing.T) {
			groupID := "cmdgrp_" + toolID
			tool := websession.HistoryTool{
				ID: toolID, Kind: "command_execution", Name: "Tool", Status: "done",
				Input: map[string]any{"command": "echo test"}, Output: "full tool output",
				CommandGroup: &websession.HistoryToolCommandGroup{ID: groupID, Count: 1, Compacted: true},
			}
			toolJSON, err := json.Marshal(tool)
			if err != nil {
				t.Fatalf("encode tool: %v", err)
			}
			sourceID := "tool:" + groupID
			row := tables.WebSessionItemTable{
				WebSessionID: session.ID, SourceItemID: &sourceID, CommandGroupID: &groupID,
				OrderIndex: int64(index + 1), ItemKind: "tool", ItemType: tool.Kind,
				ToolJSON: string(toolJSON),
			}
			row.Init()
			if err := model.GetDB().Create(&row).Error; err != nil {
				t.Fatalf("create tool history: %v", err)
			}
			for _, key := range []string{groupID, toolID} {
				payload := requestWebSessionHistoryItem(t, app, basePath+url.PathEscape(key))
				var detail websession.CommandExecutionGroupDetail
				encoded, err := json.Marshal(payload)
				if err != nil {
					t.Fatalf("encode response: %v", err)
				}
				if err := json.Unmarshal(encoded, &detail); err != nil {
					t.Fatalf("decode detail: %v", err)
				}
				if detail.GroupID != key || len(detail.Items) != 1 || detail.Items[0].ToolID != toolID {
					t.Fatalf("detail for %q lost the original tool: %#v", key, detail)
				}
				if detail.Items[0].Output != tool.Output {
					t.Fatalf("detail for %q lost the tool output: %#v", key, detail.Items[0])
				}
			}
		})
	}
}
