# 测试服务器部署与验收

当前版本可部署到测试服务器做认证、基础资料、客户价格和快速开单联调。独立收款、预存充值、退料、对账、工作台统计及后台权限管理尚未由 Go API 实现，对应前端页面暂不作为测试环境验收范围。本清单不等同于生产上线批准。

## 1. 前置条件

- Linux x86_64/arm64 服务器，安装 Docker Engine 与 Compose v2。
- 一个 HTTPS 域名，例如 `ledger-test.example.com`。
- 80/443 对外开放；PostgreSQL 端口不要对公网开放。
- 前后端两个平级项目分别上传：`claude-ledger`、`claude-ledger-backend`。

推荐同域部署：`/` 转发前端，`/api/`、`/health`、`/docs` 和 `/openapi.yaml` 转发 Go API。这样 refresh cookie 保持同站，配置最简单。

## 2. 启动数据库和 API

在后端目录执行：

```bash
cp .env.server.example .env
chmod 600 .env
```

修改 `.env` 中的域名、数据库密码和 JWT 密钥。数据库密码出现在 URL 中时必须做 URL 编码；不要使用示例值。

```bash
docker compose up -d postgres
docker compose --profile tools run --rm migrate up
docker compose build api seed
docker compose up -d api
curl --fail http://127.0.0.1:8080/health
```

需要演示数据时，仅在全新的测试数据库上执行：

```bash
docker compose --profile tools run --rm -e APP_ENV=development seed
```

Seed 账号是 `owner`、`finance`、`clerk`，密码统一为 `Paint123!`。测试结束后应禁用或删除这些账号；禁止在生产数据库执行 seed。

## 3. 构建和启动前端

在前端目录执行，API 地址必须在镜像构建时传入：

```bash
docker build \
  --build-arg NEXT_PUBLIC_API_BASE_URL=https://ledger-test.example.com/api/v1 \
  -t claude-ledger-web:test .
docker run -d --name claude-ledger-web --restart unless-stopped \
  -p 127.0.0.1:3000:3000 claude-ledger-web:test
```

反向代理必须保留 `Host`、`X-Forwarded-For`、`X-Forwarded-Proto`，并把 `/api/` 原样转发到 `127.0.0.1:8080`。HTTPS 环境保持 `APP_ENV=production` 和 `COOKIE_SECURE=true`。

## 4. 必做验收

1. `/health` 返回 200，数据库状态正常；`/docs` 可打开。
2. 三类账号登录、刷新、退出正常；停用账号和越权操作分别返回 403。
3. 油漆工、施工队、工地、材料和客户价格能查询；老板可按权限新增和修改。
4. 创建一单 ¥3000，用即时付款 ¥1000、预存抵扣 ¥500：订单未结 ¥1500，客户应收增加 ¥1500，预存余额减少 ¥500，真实资金收入仅增加 ¥1000。
5. 用同一个 `Idempotency-Key` 重放同一请求，只产生一张订单；相同 key 配不同请求返回 409。
6. 两个并发请求同时消耗接近全部预存款，只允许余额足够的请求提交，余额不得为负。
7. 重启 API 后登录会话、订单和流水仍存在；数据库备份与恢复演练成功。

第 5、6 项必须连接真实 PostgreSQL 执行，不能只以单元测试替代。通过这些项目后，本版本才算达到“测试环境可用”；在后续财务模块、权限审计、备份监控和安全测试完成前，不应承载正式账目。
