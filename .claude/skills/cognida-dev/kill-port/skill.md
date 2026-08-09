# Kill Port Skill

快速杀死占用指定端口的进程，支持 Windows、Linux 和 macOS。

## 使用方法

```
请帮我杀死 8080 端口
```

## 功能

- 自动检测操作系统
- 查找占用指定端口的进程
- 强制终止进程
- 支持批量端口处理

## 手动使用

### Windows

```bash
# 查找占用端口的进程
netstat -ano | findstr :8080

# 杀死进程
taskkill //F //PID <pid>
```

### Linux/macOS

```bash
# 查找占用端口的进程
lsof -i :8080

# 杀死进程
kill -9 <pid>
```
