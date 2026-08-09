// 工具：版本化数据库迁移执行器（golang-migrate，〔INF-4〕）。
//
// 本工具取代旧的 GORM AutoMigrate 同步方式：所有 schema 变更一律新增
// migrations/NNNNNN_*.up.sql / *.down.sql 版本化文件，由本工具驱动执行；
// 生产库与运行时（cmd/server）不再做任何自动建表/改表。
//
// 用法（先加载环境变量）：
//
//	cd services/cognida-go && set -a && source .env && set +a
//	go run ./cmd/migrate-db up            # 应用全部未执行迁移（默认动作）
//	go run ./cmd/migrate-db down [N]      # 回滚 N 步；省略 N 则回滚全部
//	go run ./cmd/migrate-db version       # 查看当前版本与 dirty 状态
//	go run ./cmd/migrate-db force <V>     # 将版本强制标记为 V（存量库接入基线用 force 1）
//
// 读取 DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME 环境变量（密码不设默认，须由环境提供）。
//
// 存量库接入：既有库已由旧 AutoMigrate 建好 32 张表，切换到版本化迁移时先
// `force 1` 把基线（000001_baseline）标记为已应用，避免重复建表；此后再 `up` 应用增量。
//
// 注意：图谱数据以 Neo4j 为唯一真源（见〔GO-3〕），MySQL 侧不保存图谱本体，迁移不涉及 graph_* 表。
package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"cognida/migrations"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: migrate-db <up|down [N]|version|force V>")
	os.Exit(2)
}

func main() {
	cmd := "up"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	// go-sql-driver DSN（非 URL 形式，密码无需 URL 编码）。
	// multiStatements=true：基线迁移单文件含多条 CREATE TABLE，需允许多语句执行。
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?multiStatements=true&parseTime=true&charset=utf8mb4&loc=Local",
		env("DB_USER", "root"),
		env("DB_PASSWORD", ""),
		env("DB_HOST", "127.0.0.1"),
		env("DB_PORT", "3306"),
		env("DB_NAME", "cognida"),
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("open db failed: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("ping db failed: %v", err)
	}

	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		log.Fatalf("load migrations failed: %v", err)
	}

	driver, err := migratemysql.WithInstance(db, &migratemysql.Config{})
	if err != nil {
		log.Fatalf("init migrate driver failed: %v", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "mysql", driver)
	if err != nil {
		log.Fatalf("init migrate failed: %v", err)
	}

	switch cmd {
	case "up":
		err = m.Up()
	case "down":
		if len(os.Args) > 2 {
			n, convErr := strconv.Atoi(os.Args[2])
			if convErr != nil || n <= 0 {
				usage()
			}
			err = m.Steps(-n)
		} else {
			err = m.Down()
		}
	case "force":
		if len(os.Args) < 3 {
			usage()
		}
		v, convErr := strconv.Atoi(os.Args[2])
		if convErr != nil {
			usage()
		}
		err = m.Force(v)
	case "version":
		v, dirty, verr := m.Version()
		if errors.Is(verr, migrate.ErrNilVersion) {
			fmt.Println("version: (none — 尚未应用任何迁移)")
			return
		}
		if verr != nil {
			log.Fatalf("read version failed: %v", verr)
		}
		fmt.Printf("version: %d  dirty: %v\n", v, dirty)
		return
	default:
		usage()
	}

	if errors.Is(err, migrate.ErrNoChange) {
		fmt.Println("OK: 无待应用变更（already up to date）")
		return
	}
	if err != nil {
		log.Fatalf("migrate %s failed: %v", cmd, err)
	}

	v, dirty, _ := m.Version()
	fmt.Printf("OK: migrate %s done — version: %d  dirty: %v\n", cmd, v, dirty)
}
