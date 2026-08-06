-- =============================================================
-- 电商演示数据源（多数据源功能演示/测试用）——引导脚本
-- 用法：mysql -h<host> -P<port> -u<root_user> -p < seed-ecommerce-demo.sql
--
-- 本脚本只做「一次性引导」：创建演示库 ecommerce_demo + 只读账号 ecommerce_ro
-- （库级 GRANT SELECT，自动覆盖后续新增表；演示环境固定密码，勿用于生产）。
--
-- 表结构与数据由 Go 种子工具统一生成（单一事实源，避免 schema 漂移）：
--   cd link-go && set -a && source .env && set +a && go run ./cmd/seed-ecommerce
-- 该工具 DROP+CREATE 全部 30 张表并灌入约 3 年跨度的关联数据（幂等，可反复执行）。
-- 重灌后务必 ANALYZE：mysqlcheck -u<root> -p --analyze ecommerce_demo
-- =============================================================

CREATE DATABASE IF NOT EXISTS ecommerce_demo
  DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- 只读账号（数据源注册用，演示环境固定密码）——库级授权覆盖 Go 工具建的全部表。
CREATE USER IF NOT EXISTS 'ecommerce_ro'@'%' IDENTIFIED BY 'EcommerceRo#2026';
GRANT SELECT ON ecommerce_demo.* TO 'ecommerce_ro'@'%';
FLUSH PRIVILEGES;
