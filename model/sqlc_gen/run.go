package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"code-kanban/model"
	"code-kanban/utils/sqlc_gen_tools"

	"gopkg.in/yaml.v3"
)

// SqlcConfig 表示 sqlc.yaml 配置文件的结构
type SqlcConfig struct {
	Version string `yaml:"version"`
	SQL     []struct {
		Engine string `yaml:"engine"`
		Schema string `yaml:"schema"`
	} `yaml:"sql"`
}

// readSqlcConfig 读取 sqlc.yaml 配置文件
func readSqlcConfig(configPath string) (*SqlcConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config SqlcConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	return &config, nil
}

// generateSchemaFile 生成指定数据库类型的 schema 文件
func generateSchemaFile(models []any, dialect, outputPath string) error {
	// 生成SQL语句
	sqlStatements, err := sqlc_gen_tools.GenerateSQLForDialect(models, dialect)
	if err != nil {
		return fmt.Errorf("生成SQL失败: %v", err)
	}

	if len(sqlStatements) == 0 {
		log.Println("警告: 没有生成任何SQL语句")
		return nil
	}

	// 添加文件头注释
	header := fmt.Sprintf("-- 数据库建表语句\n-- 生成时间: %s\n-- 数据库方言: %s\n-- 总共 %d 条语句\n\n",
		time.Now().Format("2006-01-02 15:04:05"), dialect, len(sqlStatements))

	// 格式化SQL语句
	var formattedSQL []string
	formattedSQL = append(formattedSQL, header)
	for _, sql := range sqlStatements {
		if strings.TrimSpace(sql) != "" {
			formattedSQL = append(formattedSQL, sql+";")
		} else {
			formattedSQL = append(formattedSQL, "\n")
		}
	}

	finalSQL := strings.Join(formattedSQL, "\n")

	// 写入文件
	err = os.WriteFile(outputPath, []byte(finalSQL), 0644)
	if err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}

	fmt.Printf("SQL文件已生成: %s (共 %d 条语句)\n", outputPath, len(sqlStatements))
	return nil
}

func main() {
	pathPrefix := "./model/sqlc_gen/"
	// 读取 sqlc.yaml 配置
	config, err := readSqlcConfig(filepath.Join(pathPrefix, "sqlc.yaml"))
	if err != nil {
		log.Fatalf("读取 sqlc.yaml 失败: %v", err)
	}

	if len(config.SQL) == 0 {
		log.Fatalf("sqlc.yaml 中没有找到 SQL 配置")
	}

	// 获取第一个 SQL 配置的 engine
	engine := config.SQL[0].Engine
	fmt.Printf("检测到 sqlc engine: %s\n", engine)

	models := model.GetAllModels()

	// 根据 engine 类型生成对应的 schema 文件
	switch engine {
	case "sqlite":
		err := generateSchemaFile(models, engine, filepath.Join(pathPrefix, "./schema_sqlite.sql"))
		if err != nil {
			log.Fatalf("生成 SQLite schema 失败: %v", err)
		}
	default:
		log.Fatalf("不支持的 engine 类型: %s (仅支持 sqlite)", engine)
	}

	// 运行 sqlc generate
	fmt.Println("开始运行 sqlc generate...")
	sqlc_gen_tools.RunSqlc(filepath.Join(pathPrefix, "."), filepath.Join(pathPrefix, ".."))
}
