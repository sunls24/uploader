# uploader

公开浏览和下载文件；上传、删除和一对一临时分享需要管理密码。接收者可在上传过程中开始下载。

## 启动

需要 Go 1.26+ 和 Bun，支持 Linux/macOS。先构建前端，再运行服务：

```sh
cd web
bun install --frozen-lockfile
bun --bun run build
cd ..
ADMIN_PASSWORD='替换为你的密码' go run ./cmd
```

打开 http://localhost:3000。默认文件目录为 `./fs`。

## 配置

| 环境变量 | 默认值 | 含义 |
| --- | --- | --- |
| ADMIN_PASSWORD | 必填 | 写操作管理密码 |
| STORAGE_DIR | ./fs（Docker 中为 `/data`） | 文件存储目录 |
| HOST / PORT | 空 / 3000 | 监听地址及端口 |
| SHARE_CODE_LENGTH | 8 | 数字分享码长度，允许 4～8 位，保留前导零 |
| SHARE_TTL | 30m | 分享有效期，从创建时起算 |
| MAX_FILE_BYTES | 1073741824 | 单文件上限（1 GiB） |
| MAX_SHARES | 8 | 活动共享上限，磁盘最多约占此值 × MAX_FILE_BYTES |
| COOKIE_SECURE | false | HTTPS 反向代理部署时设为 true |

## 使用与部署

- 写操作解锁 15 分钟。普通上传最多并发 3 个、最长 30 分钟；同名文件不覆盖，删除仅支持普通文件。
- 分享码用于查看和领取；领取后绑定浏览器，同一浏览器可在到期前从头重下。到期即停止传输；上传失败或服务重启后需新建分享。
- `.uploader-shares` 和 `.uploader.lock` 为保留路径；服务启动时持有目录锁并清空临时分享。强制终止可能留下 `.upload-*` 文件，可在服务停止后清理。
- 公网部署使用 HTTPS。短分享码需配合短有效期，4 位仅建议可信局域网。查询、领取和下载按直连 IP 限速；反向代理后用户可能共用限额，且代理需关闭传输缓冲并允许覆盖分享有效期的连接超时。

## Docker

```sh
docker build -t uploader .
docker run --rm -p 3000:3000 -e ADMIN_PASSWORD='替换为你的密码' -v uploader-data:/data uploader
```

## 验证

构建前端后执行 `go test -race ./...`。
