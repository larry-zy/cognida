-- =============================================================
-- 电商演示数据源（多数据源功能演示/测试用）
-- 用法：mysql -h<host> -P<port> -u<root_user> -p < seed-ecommerce-demo.sql
-- 幂等：重复执行会重建全部表与数据。
-- 同时创建只读账号 ecommerce_ro（仅 SELECT，供数据源注册使用，
-- 演示环境固定密码，勿用于生产）。
-- =============================================================

CREATE DATABASE IF NOT EXISTS ecommerce_demo
  DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE ecommerce_demo;

DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS customers;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS categories;

-- 商品分类（两级）
CREATE TABLE categories (
  id INT PRIMARY KEY AUTO_INCREMENT,
  name VARCHAR(64) NOT NULL COMMENT '分类名称',
  parent_id INT NULL COMMENT '父分类ID，NULL为一级分类'
) COMMENT='商品分类表';

-- 商品
CREATE TABLE products (
  id INT PRIMARY KEY AUTO_INCREMENT,
  category_id INT NOT NULL COMMENT '分类ID',
  name VARCHAR(128) NOT NULL COMMENT '商品名称',
  brand VARCHAR(64) NOT NULL COMMENT '品牌',
  price DECIMAL(10,2) NOT NULL COMMENT '售价',
  cost DECIMAL(10,2) NOT NULL COMMENT '成本价',
  stock INT NOT NULL DEFAULT 0 COMMENT '库存数量',
  status VARCHAR(16) NOT NULL DEFAULT 'on_sale' COMMENT '状态: on_sale在售/off_shelf下架',
  created_at DATETIME NOT NULL COMMENT '上架时间',
  KEY idx_category (category_id)
) COMMENT='商品表';

-- 客户
CREATE TABLE customers (
  id INT PRIMARY KEY AUTO_INCREMENT,
  name VARCHAR(64) NOT NULL COMMENT '客户姓名',
  gender VARCHAR(8) NOT NULL COMMENT '性别: male/female',
  city VARCHAR(32) NOT NULL COMMENT '所在城市',
  vip_level TINYINT NOT NULL DEFAULT 0 COMMENT 'VIP等级 0-3',
  email VARCHAR(128) NOT NULL COMMENT '邮箱',
  phone VARCHAR(20) NOT NULL COMMENT '手机号',
  registered_at DATETIME NOT NULL COMMENT '注册时间'
) COMMENT='客户表';

-- 订单
CREATE TABLE orders (
  id INT PRIMARY KEY AUTO_INCREMENT,
  order_no VARCHAR(32) NOT NULL UNIQUE COMMENT '订单号',
  customer_id INT NOT NULL COMMENT '客户ID',
  status VARCHAR(16) NOT NULL COMMENT '状态: completed完成/shipped已发货/paid已支付/cancelled已取消/refunded已退款',
  total_amount DECIMAL(12,2) NOT NULL DEFAULT 0 COMMENT '订单总额',
  discount_amount DECIMAL(12,2) NOT NULL DEFAULT 0 COMMENT '优惠金额',
  pay_amount DECIMAL(12,2) NOT NULL DEFAULT 0 COMMENT '实付金额',
  payment_method VARCHAR(16) NOT NULL COMMENT '支付方式: alipay/wechat/card',
  created_at DATETIME NOT NULL COMMENT '下单时间',
  paid_at DATETIME NULL COMMENT '支付时间，未支付/取消为NULL',
  KEY idx_customer (customer_id),
  KEY idx_created (created_at)
) COMMENT='订单表';

-- 订单明细
CREATE TABLE order_items (
  id INT PRIMARY KEY AUTO_INCREMENT,
  order_id INT NOT NULL COMMENT '订单ID',
  product_id INT NOT NULL COMMENT '商品ID',
  quantity INT NOT NULL COMMENT '购买数量',
  unit_price DECIMAL(10,2) NOT NULL COMMENT '成交单价',
  subtotal DECIMAL(12,2) NOT NULL COMMENT '小计',
  KEY idx_order (order_id),
  KEY idx_product (product_id)
) COMMENT='订单明细表';

-- ---------------- 分类 ----------------
INSERT INTO categories (id, name, parent_id) VALUES
  (1, '数码电子', NULL),
  (2, '家用电器', NULL),
  (3, '服饰鞋包', NULL),
  (4, '美妆个护', NULL),
  (5, '食品饮料', NULL),
  (6, '运动户外', NULL),
  (7, '手机', 1), (8, '笔记本电脑', 1), (9, '耳机音箱', 1),
  (10, '冰箱洗衣机', 2), (11, '厨房小电', 2),
  (12, '男装', 3), (13, '女装', 3), (14, '箱包', 3);

-- ---------------- 商品（40个，手写真实感数据） ----------------
INSERT INTO products (category_id, name, brand, price, cost, stock, status, created_at) VALUES
  (7,  '小米14 Pro 512G 钛金属', '小米', 4999.00, 3800.00, 320, 'on_sale',  NOW() - INTERVAL 200 DAY),
  (7,  'iPhone 15 128G 蓝色', 'Apple', 5999.00, 4600.00, 150, 'on_sale',  NOW() - INTERVAL 260 DAY),
  (7,  '华为 Mate 60 Pro 曜金黑', '华为', 6999.00, 5300.00, 88,  'on_sale',  NOW() - INTERVAL 240 DAY),
  (7,  'Redmi K70 16G+512G', '小米', 2699.00, 2000.00, 500, 'on_sale',  NOW() - INTERVAL 150 DAY),
  (7,  'OPPO Find X7 Ultra', 'OPPO', 5999.00, 4500.00, 60,  'on_sale',  NOW() - INTERVAL 120 DAY),
  (7,  'vivo X100 白月光', 'vivo', 3999.00, 3000.00, 0,   'off_shelf', NOW() - INTERVAL 300 DAY),
  (8,  'MacBook Air M3 16G/512G', 'Apple', 9499.00, 7500.00, 45,  'on_sale',  NOW() - INTERVAL 180 DAY),
  (8,  '联想小新 Pro16 酷睿Ultra5', '联想', 5499.00, 4300.00, 120, 'on_sale',  NOW() - INTERVAL 160 DAY),
  (8,  '华为 MateBook X Pro', '华为', 8999.00, 7000.00, 30,  'on_sale',  NOW() - INTERVAL 140 DAY),
  (8,  'ROG 幻16 Air 游戏本', '华硕', 12999.00, 10200.00, 15, 'on_sale',  NOW() - INTERVAL 90 DAY),
  (9,  'AirPods Pro 2 USB-C', 'Apple', 1899.00, 1300.00, 400, 'on_sale',  NOW() - INTERVAL 220 DAY),
  (9,  '索尼 WH-1000XM5 头戴降噪', '索尼', 2399.00, 1700.00, 180, 'on_sale',  NOW() - INTERVAL 250 DAY),
  (9,  '小米真无线降噪耳机4 Pro', '小米', 699.00, 420.00, 800, 'on_sale',  NOW() - INTERVAL 100 DAY),
  (9,  'Bose SoundLink 蓝牙音箱', 'Bose', 1299.00, 900.00, 90,  'on_sale',  NOW() - INTERVAL 170 DAY),
  (10, '海尔 501L 十字对开门冰箱', '海尔', 3599.00, 2700.00, 40,  'on_sale',  NOW() - INTERVAL 280 DAY),
  (10, '小天鹅 10KG 滚筒洗衣机', '小天鹅', 2799.00, 2000.00, 55,  'on_sale',  NOW() - INTERVAL 230 DAY),
  (10, '美的 452L 法式多门冰箱', '美的', 2999.00, 2200.00, 35,  'on_sale',  NOW() - INTERVAL 190 DAY),
  (11, '九阳破壁机 Y88', '九阳', 599.00, 350.00, 260, 'on_sale',  NOW() - INTERVAL 130 DAY),
  (11, '小熊电煮锅 1.5L', '小熊', 129.00, 70.00,  900, 'on_sale',  NOW() - INTERVAL 110 DAY),
  (11, '摩飞多功能料理锅', '摩飞', 899.00, 560.00, 75,  'on_sale',  NOW() - INTERVAL 210 DAY),
  (12, '优衣库男装摇粒绒外套', '优衣库', 199.00, 90.00,  1500,'on_sale',  NOW() - INTERVAL 160 DAY),
  (12, '海澜之家商务休闲西裤', '海澜之家', 299.00, 130.00, 600, 'on_sale',  NOW() - INTERVAL 140 DAY),
  (12, '北面男款冲锋衣 1990', '北面', 1299.00, 700.00, 200, 'on_sale',  NOW() - INTERVAL 120 DAY),
  (13, 'ONLY 法式碎花连衣裙', 'ONLY', 399.00, 160.00, 350, 'on_sale',  NOW() - INTERVAL 100 DAY),
  (13, '波司登轻薄羽绒服女', '波司登', 899.00, 480.00, 280, 'on_sale',  NOW() - INTERVAL 250 DAY),
  (13, '太平鸟针织开衫', '太平鸟', 259.00, 110.00, 420, 'on_sale',  NOW() - INTERVAL 80 DAY),
  (14, '新秀丽 20寸拉杆箱', '新秀丽', 1099.00, 620.00, 130, 'on_sale',  NOW() - INTERVAL 200 DAY),
  (14, 'Coach 单肩托特包', 'Coach', 2499.00, 1500.00, 25,  'on_sale',  NOW() - INTERVAL 170 DAY),
  (4,  '兰蔻小黑瓶精华 50ml', '兰蔻', 1080.00, 600.00, 220, 'on_sale',  NOW() - INTERVAL 190 DAY),
  (4,  '雅诗兰黛白金面霜', '雅诗兰黛', 1850.00, 1050.00, 60, 'on_sale',  NOW() - INTERVAL 220 DAY),
  (4,  '薇诺娜特护霜 50g', '薇诺娜', 268.00, 120.00, 700, 'on_sale',  NOW() - INTERVAL 90 DAY),
  (4,  '飞利浦电动牙刷 HX6853', '飞利浦', 399.00, 210.00, 380, 'on_sale',  NOW() - INTERVAL 150 DAY),
  (5,  '三只松鼠坚果大礼包 30袋', '三只松鼠', 128.00, 65.00, 2000,'on_sale', NOW() - INTERVAL 60 DAY),
  (5,  '农夫山泉东方树叶 15瓶整箱', '农夫山泉', 67.00, 38.00, 3000,'on_sale', NOW() - INTERVAL 70 DAY),
  (5,  '瑞幸咖啡冻干粉 2g*24', '瑞幸', 89.00, 45.00, 1200,'on_sale',  NOW() - INTERVAL 50 DAY),
  (5,  '五常大米 10斤装', '十月稻田', 79.00, 48.00, 800, 'on_sale',  NOW() - INTERVAL 40 DAY),
  (6,  '迪卡侬折叠自行车 20寸', '迪卡侬', 1499.00, 950.00, 45,  'on_sale',  NOW() - INTERVAL 130 DAY),
  (6,  'Keep 智能动感单车 C1', 'Keep', 2199.00, 1400.00, 30,  'on_sale',  NOW() - INTERVAL 180 DAY),
  (6,  '李宁羽毛球拍套装', '李宁', 329.00, 170.00, 260, 'on_sale',  NOW() - INTERVAL 110 DAY),
  (6,  '斯伯丁篮球 7号 TF传奇', '斯伯丁', 249.00, 130.00, 340, 'on_sale',  NOW() - INTERVAL 95 DAY);

-- ---------------- 客户（50个，程序化生成） ----------------
INSERT INTO customers (name, gender, city, vip_level, email, phone, registered_at)
WITH RECURSIVE seq AS (
  SELECT 1 AS n UNION ALL SELECT n + 1 FROM seq WHERE n < 50
)
SELECT
  CONCAT(ELT(1 + (n % 10), '王','李','张','刘','陈','杨','赵','黄','周','吴'),
         ELT(1 + (n % 12), '伟','芳','娜','敏','静','磊','军','洋','勇','艳','杰','涛')),
  IF(n % 2 = 0, 'female', 'male'),
  ELT(1 + (n % 8), '北京','上海','广州','深圳','杭州','成都','武汉','南京'),
  n % 4,
  CONCAT('user', LPAD(n, 3, '0'), '@example.com'),
  CONCAT('138', LPAD(n * 137 % 100000000, 8, '0')),
  NOW() - INTERVAL (n * 7 + 30) DAY
FROM seq;

-- ---------------- 订单（300个，近180天） ----------------
INSERT INTO orders (order_no, customer_id, status, total_amount, discount_amount, pay_amount, payment_method, created_at, paid_at)
WITH RECURSIVE seq AS (
  SELECT 1 AS n UNION ALL SELECT n + 1 FROM seq WHERE n < 300
)
SELECT
  CONCAT('SO', LPAD(n, 8, '0')),
  1 + (n * 7 % 50),
  ELT(1 + CASE WHEN n % 20 < 11 THEN 0 WHEN n % 20 < 14 THEN 1
               WHEN n % 20 < 17 THEN 2 WHEN n % 20 < 19 THEN 3 ELSE 4 END,
      'completed', 'shipped', 'paid', 'cancelled', 'refunded'),
  0, 0, 0,
  ELT(1 + (n % 3), 'alipay', 'wechat', 'card'),
  NOW() - INTERVAL ((n * 37) % 180) DAY - INTERVAL ((n * 53) % 86400) SECOND,
  NULL
FROM seq;

-- ---------------- 订单明细（每单1~3件） ----------------
INSERT INTO order_items (order_id, product_id, quantity, unit_price, subtotal)
SELECT
  o.id,
  p.id,
  1 + ((o.id + s.n) % 3),
  p.price,
  p.price * (1 + ((o.id + s.n) % 3))
FROM orders o
JOIN (SELECT 1 AS n UNION SELECT 2 UNION SELECT 3) s
  ON s.n <= 1 + (o.id % 3)
JOIN products p
  ON p.id = 1 + ((o.id * 13 + s.n * 7) % 40);

-- 回填订单金额（总额=明细合计，优惠按0/2%/4%/6%档）
UPDATE orders o
JOIN (SELECT order_id, SUM(subtotal) AS t FROM order_items GROUP BY order_id) x
  ON o.id = x.order_id
SET o.total_amount   = x.t,
    o.discount_amount = ROUND(x.t * (o.id % 4) * 0.02, 2),
    o.pay_amount      = x.t - ROUND(x.t * (o.id % 4) * 0.02, 2);

-- 已支付类订单回填支付时间（取消单保持 NULL）
UPDATE orders
SET paid_at = created_at + INTERVAL (id % 30 + 1) MINUTE
WHERE status IN ('completed', 'shipped', 'paid', 'refunded');

-- ---------------- 只读账号（数据源注册用，演示环境固定密码） ----------------
CREATE USER IF NOT EXISTS 'ecommerce_ro'@'%' IDENTIFIED BY 'EcommerceRo#2026';
GRANT SELECT ON ecommerce_demo.* TO 'ecommerce_ro'@'%';
FLUSH PRIVILEGES;
