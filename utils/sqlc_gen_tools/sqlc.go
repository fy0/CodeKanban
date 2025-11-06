package sqlc_gen_tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// runSQLC 在指定目录内执行 sqlc generate。
func RunSqlc(sqlcDir string, modelDir string) {
	if sqlcDir == "" {
		sqlcDir = "."
	}

	if _, err := os.Stat(sqlcDir); os.IsNotExist(err) {
		fmt.Printf("提示: %s 目录不存在，跳过 sqlc generate\n", sqlcDir)
		return
	}

	sqlcConfig := filepath.Join(sqlcDir, "sqlc.yaml")
	if _, err := os.Stat(sqlcConfig); os.IsNotExist(err) {
		fmt.Println("提示: 未找到 sqlc.yaml，跳过 sqlc generate")
		return
	}

	cmdNames := []string{"sqlc", "./sqlc", "sqlc.exe", "./sqlc.exe"}
	var cmdPath string

	for _, name := range cmdNames {
		if abs, err := exec.LookPath(name); err == nil {
			cmdPath, _ = filepath.Abs(abs)
			break
		}
	}

	if cmdPath == "" {
		fmt.Println("提示: 未找到 sqlc 可执行文件，请将 sqlc 加入 PATH 或放在当前目录")
		return
	}

	cmd := exec.Command(cmdPath, "generate")
	cmd.Dir = sqlcDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("sqlc generate 执行失败: %v\n", err)
		return
	}

	fmt.Println("sqlc generate 执行完成")
}
