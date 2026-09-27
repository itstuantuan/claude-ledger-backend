# 云记账 Go 后端

这是与 `../claude-ledger` Next.js 前端完全分离的独立 Go 服务。

当前进度：Phase 1–4。已实现平台基础、认证/RBAC、基础资料，以及快速开单的订单快照、订单内补料/退料流水、并发业务编号、持久幂等、Ledger、即时付款、预存抵扣、真实资金流水、审计和账户不变量防线。独立收款与预存充值将在后续阶段实现。

设计依据来自前端项目的 `docs/`：

- `frontend-contract-analysis.md`
- `api-contract.md`
- `accounting-rules.md`
- `database.md`
- `architecture.md`

开发说明见 `docs/development.md`。
