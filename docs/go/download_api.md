# 文件下载 API

## 概述

文件下载功能提供本地文件下载、文件管理、知识库导出等能力。

**下载目录**: `D:\link\download`

## API 接口

所有接口需要认证（Bearer Token + X-Tenant-ID）。

### 1. 列出已下载的文件

**接口**: `GET /api/v1/download/list`

**响应示例**:
```json
{
  "message": "success",
  "data": [
    {
      "name": "example.txt",
      "size": 1024,
      "modTime": "2024-02-12T10:30:00Z",
      "isDir": false
    }
  ],
  "count": 1
}
```

### 2. 下载文件

**接口**: `GET /api/v1/download/file?filepath=<path>`

**参数**:
- `filepath`: 文件相对路径（相对于 D:\link 目录）

**示例**:
```bash
curl -X GET "http://localhost:8080/api/v1/download/file?filepath=knowledge\\test.txt" \
  -H "Authorization: Bearer <token>" \
  -H "X-Tenant-ID: 1" \
  -O downloaded.txt
```

### 3. 创建本地文件

**接口**: `POST /api/v1/download/local?filename=<name>`

**参数**:
- `filename`: 文件名

**响应示例**:
```json
{
  "message": "file created successfully",
  "path": "D:\\link\\download\\example.txt",
  "filename": "example.txt",
  "size": 256
}
```

**注意**: 当前实现为生成示例文件，后续可扩展为从知识库导出。

### 4. 删除已下载的文件

**接口**: `DELETE /api/v1/download/:filename`

**示例**:
```bash
curl -X DELETE "http://localhost:8080/api/v1/download/example.txt" \
  -H "Authorization: Bearer <token>" \
  -H "X-Tenant-ID: 1"
```

**响应**:
```json
{
  "message": "file deleted successfully",
  "filename": "example.txt"
}
```

### 5. 批量下载

**接口**: `POST /api/v1/download/batch`

**请求体**:
```json
{
  "files": ["file1.txt", "file2.txt"]
}
```

**响应**:
```json
{
  "message": "batch download not yet implemented",
  "files": ["file1.txt", "file2.txt"],
  "count": 2,
  "note": "will be implemented later with zip packaging"
}
```

### 6. 导出知识库

**接口**: `POST /api/v1/download/export?format=<format>`

**参数**:
- `format`: 导出格式 (txt, json, markdown)

**请求体**:
```json
{
  "title": "我的知识库",
  "entities": [
    {"name": "实体1", "type": "Person"},
    {"name": "实体2", "type": "Organization"}
  ],
  "relations": [
    {"source": "实体1", "target": "实体2", "description": "关系描述"}
  ]
}
```

**响应**:
```json
{
  "message": "knowledge exported successfully",
  "filename": "knowledge_export.md",
  "path": "D:\\link\\download\\knowledge_export.md",
  "format": "markdown",
  "entities": 2,
  "relations": 1
}
```

## 使用示例

### 列出下载目录

```bash
curl -X GET "http://localhost:8080/api/v1/download/list" \
  -H "Authorization: Bearer <your_token>" \
  -H "X-Tenant-ID: 1"
```

### 创建示例文件

```bash
curl -X POST "http://localhost:8080/api/v1/download/local?filename=test.md" \
  -H "Authorization: Bearer <your_token>" \
  -H "X-Tenant-ID: 1"
```

### 导出知识库为 Markdown

```bash
curl -X POST "http://localhost:8080/api/v1/download/export?format=markdown" \
  -H "Authorization: Bearer <your_token>" \
  -H "X-Tenant-ID: 1" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "我的知识库",
    "entities": [{"name": "实体1"}],
    "relations": []
  }'
```

## 安全特性

1. **路径遍历防护**: 检测并拒绝包含 `..` 的文件名
2. **认证要求**: 所有接口都需要有效的 JWT Token
3. **文件类型检查**: 基于扩展名设置正确的 Content-Type

## 后续扩展方向

1. **远程下载**: 从 HTTP/FTP 服务器下载文件
2. **ZIP 打包**: 批量下载时打包为 ZIP 文件
3. **下载历史**: 记录下载历史，支持重新下载
4. **进度跟踪**: 大文件下载进度显示
5. **断点续传**: 支持大文件断点下载
6. **权限控制**: 细粒度的文件访问权限管理

## 文件结构

```
internal/handler/download.go    # 下载处理器
D:\link\download\              # 本地下载目录
```
