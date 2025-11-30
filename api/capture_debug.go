package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gofiber/fiber/v2"
	"github.com/tuzig/vt10x"
	"go.uber.org/zap"

	"code-kanban/service/terminal"
	"code-kanban/utils/ai_assistant2"
)

const (
	captureDefaultRows = 24
	captureDefaultCols = 80
	captureMaxRows     = 120
	captureMaxCols     = 240
	captureFallbackFG  = "#e8eaed"
	captureFallbackBG  = "#1c1f24"
)

var captureDebugTemplate = template.Must(template.New("capture-debug").Parse(captureDebugTemplateHTML))

const captureDebugTemplateHTML = `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="utf-8"/>
<title>Capture Debug</title>
<style>
body{font-family:Consolas,Menlo,monospace;background:#0b0d11;color:#e6e8ec;margin:0;padding:18px;}
h1{font-size:20px;margin:0 0 10px;}
.source{font-size:14px;margin-bottom:12px;opacity:0.85;}
.stats{font-size:13px;margin:4px 0 14px;opacity:.7;}
.message{margin:12px 0;padding:10px;background:#24160f;border-left:4px solid #f2a65a;font-size:14px;}
.empty{margin-top:16px;font-size:14px;opacity:.8;}
.grid{display:grid;grid-template-columns:repeat({{.Cols}}, minmax(12px, 1fr));border:1px solid #222;}
.cell{display:flex;align-items:center;justify-content:center;font-size:13px;padding:2px;min-height:20px;cursor:pointer;border:0.5px solid rgba(255,255,255,.05);}
.cell:hover{outline:1px solid #4da3ff;z-index:2;}
.footer{margin-top:18px;padding-top:12px;border-top:1px solid #2c2f36;font-size:14px;}
.color-preview{margin-top:8px;display:flex;gap:18px;align-items:center;flex-wrap:wrap;}
.color-box{width:46px;height:18px;border:1px solid #555;display:inline-block;margin:0 6px;}
.color-code{font-size:12px;letter-spacing:0.5px;opacity:0.8;}
</style>
</head>
<body>
<h1>/capture-debug</h1>
<div class="source">{{.Source}}</div>
{{if .Stats}}<div class="stats">{{.Stats}}</div>{{end}}
{{if .Message}}<div class="message">{{.Message}}</div>{{end}}
{{if .HasGrid}}
<div class="grid">
{{range .Grid}}
    {{range .}}
        <span class="cell"
              data-row="{{.Row}}"
              data-col="{{.Col}}"
              data-code="{{.Code}}"
              data-mode="{{.Mode}}"
              data-char="{{.Char}}"
              data-label="{{.Label}}"
              data-fg="{{.FG}}"
              data-fg-raw="{{.FGRaw}}"
              data-bg="{{.BG}}"
              data-bg-raw="{{.BGRaw}}"
              style="color: {{.FG}}; background-color: {{.BG}};">
            {{.Display}}
        </span>
    {{end}}
{{end}}
</div>
<div class="footer">
    <div id="cell-info">点击任意格子查看详细信息。</div>
    <div class="color-preview">
        <div>前景<span class="color-box" id="fg-preview"></span><span class="color-code" id="fg-code">--</span> <span class="color-code" id="fg-raw">(原值--)</span></div>
        <div>背景<span class="color-box" id="bg-preview"></span><span class="color-code" id="bg-code">--</span> <span class="color-code" id="bg-raw">(原值--)</span></div>
    </div>
</div>
{{else}}
<div class="empty">例如：/capture-debug?sessionId=xxx 或 /capture-debug?data=BASE64&rows=30&cols=120</div>
{{end}}
<script>
(function(){
    var info=document.getElementById('cell-info');
    var fg=document.getElementById('fg-preview');
    var bg=document.getElementById('bg-preview');
    var fgText=document.getElementById('fg-code');
    var bgText=document.getElementById('bg-code');
    var fgRaw=document.getElementById('fg-raw');
    var bgRaw=document.getElementById('bg-raw');
    document.querySelectorAll('.cell').forEach(function(cell){
        cell.addEventListener('click',function(){
            var payload={
                row:cell.dataset.row,
                col:cell.dataset.col,
                code:cell.dataset.code,
                mode:cell.dataset.mode,
                char:cell.dataset.char,
                label:cell.dataset.label,
                fg:cell.dataset.fg,
                fgRaw:cell.dataset.fgRaw,
                bg:cell.dataset.bg,
                bgRaw:cell.dataset.bgRaw
            };
            var shown=payload.label||payload.char||'(空)';
            info.textContent='Row '+payload.row+', Col '+payload.col+' => '+shown+' ['+payload.code+'] Mode '+payload.mode;
            fg.style.backgroundColor=payload.fg||'transparent';
            bg.style.backgroundColor=payload.bg||'transparent';
            fgText.textContent=payload.fg||'--';
            bgText.textContent=payload.bg||'--';
            fgRaw.textContent='(原值 '+(payload.fgRaw||'--')+')';
            bgRaw.textContent='(原值 '+(payload.bgRaw||'--')+')';
            console.log('capture-cell',payload);
        });
    });
})();
</script>
</body>
</html>`

type captureDebugCell struct {
	Row     int
	Col     int
	Mode    int
	FG      string
	FGRaw   string
	BG      string
	BGRaw   string
	Char    string
	Label   string
	Code    string
	Display template.HTML
}

type captureDebugPage struct {
	Rows    int
	Cols    int
	Source  string
	Message string
	Stats   string
	HasGrid bool
	Grid    [][]captureDebugCell
}

func registerCaptureDebugRoute(app *fiber.App, manager *terminal.Manager, logger *zap.Logger) {
	if manager == nil {
		return
	}

	app.Get("/capture-debug", func(c *fiber.Ctx) error {
		page := captureDebugPage{
			Rows:   captureDefaultRows,
			Cols:   captureDefaultCols,
			Source: "传入 sessionId 或 data 参数后查看捕获内容。",
		}

		rows, rowsProvided, err := parseBoundedInt(c.Query("rows"), captureDefaultRows, 1, captureMaxRows)
		if err != nil {
			return fiber.NewError(http.StatusBadRequest, "rows 需要为正整数")
		}
		cols, colsProvided, err := parseBoundedInt(c.Query("cols"), captureDefaultCols, 1, captureMaxCols)
		if err != nil {
			return fiber.NewError(http.StatusBadRequest, "cols 需要为正整数")
		}
		page.Rows = rows
		page.Cols = cols

		rawData := strings.TrimSpace(c.Query("data"))
		sessionID := strings.TrimSpace(c.Query("sessionId"))
		timeout := parseCaptureTimeout(c.Query("timeout"))
		trimView := !parseBool(c.Query("full"), false)

		var chunkBytes []byte
		var chunkSource string

		switch {
		case rawData != "":
			decoded, decodeErr := base64.StdEncoding.DecodeString(rawData)
			if decodeErr == nil {
				chunkBytes = decoded
				chunkSource = fmt.Sprintf("来自 data 参数（base64，%d 字节）", len(chunkBytes))
			} else {
				chunkBytes = []byte(rawData)
				chunkSource = fmt.Sprintf("来自 data 参数（UTF-8，%d 字节，base64 解码失败：%v）", len(chunkBytes), decodeErr)
			}
		case sessionID != "":
			session, err := manager.GetSession(sessionID)
			if err != nil {
				page.Message = fmt.Sprintf("无法找到 session %s：%v", sessionID, err)
				return renderCaptureDebugPage(c, page)
			}
			snap := session.Snapshot()
			if !rowsProvided && snap.Rows > 0 {
				page.Rows = clampInt(snap.Rows, 1, captureMaxRows)
			}
			if !colsProvided && snap.Cols > 0 {
				page.Cols = clampInt(snap.Cols, 1, captureMaxCols)
			}

			chunk, err := manager.CaptureChunk(context.Background(), sessionID, timeout)
			if err != nil {
				if logger != nil {
					logger.Warn("failed to capture terminal chunk",
						zap.String("sessionId", sessionID),
						zap.Error(err),
					)
				}
				page.Message = fmt.Sprintf("捕获 session %s 数据失败：%v", sessionID, err)
				return renderCaptureDebugPage(c, page)
			}
			chunkBytes = chunk.Data
			chunkSource = fmt.Sprintf("session %s 捕获：%d 字节 @ %s", sessionID, len(chunkBytes), chunk.Timestamp.Format(time.RFC3339))
		default:
			page.Message = "示例：/capture-debug?sessionId=xxx 或 /capture-debug?data=BASE64&rows=30&cols=120"
			return renderCaptureDebugPage(c, page)
		}

		if len(chunkBytes) == 0 {
			page.Message = "捕获数据为空。"
			return renderCaptureDebugPage(c, page)
		}

		grid := ai_assistant2.RenderGlyphGridFromBuffer(chunkBytes, page.Rows, page.Cols)
		originalRows := len(grid)
		if originalRows == 0 {
			page.Message = "未能渲染任何网格（请检查行列参数）。"
			return renderCaptureDebugPage(c, page)
		}
		originalCols := len(grid[0])
		if originalCols == 0 {
			page.Message = "网格列数无效。"
			return renderCaptureDebugPage(c, page)
		}

		effectiveRows := originalRows
		effectiveCols := originalCols
		if trimView {
			if trimmed, rowsUsed, colsUsed := shrinkGlyphGrid(grid); rowsUsed > 0 && colsUsed > 0 && (rowsUsed < effectiveRows || colsUsed < effectiveCols) {
				grid = trimmed
				effectiveRows = rowsUsed
				effectiveCols = colsUsed
			}
		}

		page.Grid = convertGlyphGrid(grid)
		page.HasGrid = true
		page.Source = chunkSource
		page.Rows = effectiveRows
		page.Cols = effectiveCols
		page.Stats = buildGridStats(originalRows, originalCols, effectiveRows, effectiveCols, trimView && (effectiveRows != originalRows || effectiveCols != originalCols))

		return renderCaptureDebugPage(c, page)
	})
}

func renderCaptureDebugPage(c *fiber.Ctx, page captureDebugPage) error {
	var buf bytes.Buffer
	if err := captureDebugTemplate.Execute(&buf, page); err != nil {
		return fiber.NewError(http.StatusInternalServerError, "failed to render capture debug page")
	}
	c.Type("html", "utf-8")
	return c.Send(buf.Bytes())
}

func parseBoundedInt(raw string, fallback, min, max int) (int, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, false, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback, false, err
	}
	return clampInt(value, min, max), true, nil
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func parseCaptureTimeout(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 2 * time.Second
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 2 * time.Second
	}
	value = clampInt(value, 1, 10)
	return time.Duration(value) * time.Second
}

func convertGlyphGrid(grid [][]vt10x.Glyph) [][]captureDebugCell {
	rows := len(grid)
	result := make([][]captureDebugCell, rows)
	for r := 0; r < rows; r++ {
		row := grid[r]
		cells := make([]captureDebugCell, len(row))
		for c := 0; c < len(row); c++ {
			glyph := row[c]
			display := glyphToDisplay(glyph.Char)
			char, label := glyphCharValues(glyph.Char)
			cells[c] = captureDebugCell{
				Row:     r,
				Col:     c,
				Mode:    int(glyph.Mode),
				FG:      colorToCSS(glyph.FG, captureFallbackFG),
				FGRaw:   colorRawLabel(glyph.FG),
				BG:      colorToCSS(glyph.BG, captureFallbackBG),
				BGRaw:   colorRawLabel(glyph.BG),
				Char:    char,
				Label:   label,
				Code:    fmt.Sprintf("U+%04X", glyph.Char),
				Display: display,
			}
		}
		result[r] = cells
	}
	return result
}

func glyphToDisplay(r rune) template.HTML {
	switch {
	case r == 0:
		return template.HTML("&#xa0;")
	case r == ' ':
		return template.HTML("&nbsp;")
	case unicode.IsControl(r):
		return template.HTML("&#xa0;")
	default:
		return template.HTML(html.EscapeString(string(r)))
	}
}

func glyphCharValues(r rune) (string, string) {
	switch r {
	case 0:
		return "", "(NUL)"
	case '\n':
		return "", "\\n"
	case '\r':
		return "", "\\r"
	case '\t':
		return "", "\\t"
	case ' ':
		return " ", "(space)"
	}
	if unicode.IsControl(r) {
		return "", fmt.Sprintf("CTRL-%d", r)
	}
	return string(r), string(r)
}

var ansiColorHex = [...]string{
	"#000000", "#cd3131", "#0dbc79", "#e5e510",
	"#2472c8", "#bc3fbc", "#11a8cd", "#e5e5e5",
	"#666666", "#f14c4c", "#23d18b", "#f5f543",
	"#3b8eea", "#d670d6", "#29b8db", "#ffffff",
}

const colorMaxRGB = 0xFFFFFF

func colorToCSS(color vt10x.Color, fallback string) string {
	switch color {
	case vt10x.DefaultFG, vt10x.DefaultBG, vt10x.DefaultCursor:
		return fallback
	}

	value := int(color)
	if value >= 0 && value <= 255 {
		if value < len(ansiColorHex) {
			return ansiColorHex[value]
		}
		if value >= 16 && value <= 231 {
			index := value - 16
			r := index / 36
			g := (index % 36) / 6
			b := index % 6
			return fmt.Sprintf("#%02x%02x%02x", colorComponent(r), colorComponent(g), colorComponent(b))
		}
		if value >= 232 && value <= 255 {
			level := value - 232
			v := 8 + level*10
			return fmt.Sprintf("#%02x%02x%02x", v, v, v)
		}
	}

	if value >= 0 && value <= colorMaxRGB {
		return fmt.Sprintf("#%06x", value&colorMaxRGB)
	}

	return fallback
}

func colorComponent(component int) int {
	if component <= 0 {
		return 0
	}
	n := 55 + component*40
	if n > 255 {
		return 255
	}
	return n
}

func colorRawLabel(color vt10x.Color) string {
	value := int(color)
	hex := fmt.Sprintf("0x%X", value)
	switch color {
	case vt10x.DefaultFG:
		return fmt.Sprintf("DefaultFG (%s)", hex)
	case vt10x.DefaultBG:
		return fmt.Sprintf("DefaultBG (%s)", hex)
	case vt10x.DefaultCursor:
		return fmt.Sprintf("DefaultCursor (%s)", hex)
	}
	if value >= 0 && value < 16 {
		return fmt.Sprintf("ANSI-%d (%s)", value, hex)
	}
	if value >= 16 && value <= 255 {
		return fmt.Sprintf("XTERM-%d (%s)", value, hex)
	}
	if value >= 0 && value <= colorMaxRGB {
		return fmt.Sprintf("RGB-#%06X (%s)", value, hex)
	}
	return fmt.Sprintf("%d (%s)", value, hex)
}

func shrinkGlyphGrid(grid [][]vt10x.Glyph) ([][]vt10x.Glyph, int, int) {
	maxRow := -1
	maxCol := -1
	for r, row := range grid {
		for c, glyph := range row {
			if glyphOccupied(glyph) {
				if r > maxRow {
					maxRow = r
				}
				if c > maxCol {
					maxCol = c
				}
			}
		}
	}
	if maxRow < 0 || maxCol < 0 {
		return nil, 0, 0
	}
	trimmed := make([][]vt10x.Glyph, maxRow+1)
	for r := 0; r <= maxRow; r++ {
		row := grid[r]
		if len(row) == 0 {
			continue
		}
		limit := maxCol + 1
		if limit > len(row) {
			limit = len(row)
		}
		trimmed[r] = make([]vt10x.Glyph, limit)
		copy(trimmed[r], row[:limit])
	}
	return trimmed, maxRow + 1, maxCol + 1
}

func glyphOccupied(glyph vt10x.Glyph) bool {
	if glyph.Char != 0 {
		return true
	}
	if glyph.Mode != 0 {
		return true
	}
	if glyph.FG != vt10x.DefaultFG || glyph.BG != vt10x.DefaultBG {
		return true
	}
	return false
}

func parseBool(raw string, fallback bool) bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func buildGridStats(originalRows, originalCols, rows, cols int, trimmed bool) string {
	if originalRows == 0 || originalCols == 0 {
		return ""
	}
	if trimmed {
		return fmt.Sprintf("终端网格：%d×%d → 当前展示 %d×%d（full=1 查看完整网格）", originalRows, originalCols, rows, cols)
	}
	return fmt.Sprintf("终端网格：%d×%d", rows, cols)
}
