# 本地开发

## 前置条件

- Go 1.26+
- Docker 与 Docker Compose

## 启动

```bash
cp .env.example .env
docker compose up -d postgres
docker compose --profile tools run --rm migrate up
set -a; source .env; set +a
go run ./cmd/server
```

访问 `http://localhost:8080/health`。前端项目位于平级目录 `../claude-ledger`，设置：

```dotenv
NEXT_PUBLIC_API_MODE=real
NEXT_PUBLIC_API_BASE_URL=http://localhost:8080/api/v1
```

当前已提供认证、基础资料和快速开单 API。首次开发环境可执行：

```bash
go run ./cmd/seed
```

这会幂等创建示范门店，`owner`、`finance`、`clerk` 三个账号，以及 3 个施工队、10 个油漆工、5 个工地、24 个材料和客户价格。首位油漆工还会获得一组平衡的 ¥2000 预存账户、预存流水和真实资金收入，用于测试开单抵扣。开发密码均为 `Paint123!`。Seed 在 production 环境会拒绝运行。API 合同可访问 `/openapi.yaml`，说明页为 `/docs`。

## 验证

```bash
make fmt-check
make vet
make test
docker compose config
```

Migration 由 golang-migrate 管理。应用启动不会执行 GORM AutoMigrate。

测试服务器部署步骤见 `test-deployment.md`。
