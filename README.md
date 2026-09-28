# 云记账 Go 后端

这是与 `../claude-ledger` Next.js 前端完全分离的独立 Go 服务。

当前进度：Phase 1–5（部分）。已实现平台基础、认证/RBAC、基础资料、快速开单、订单内补料/退料，以及独立收款的自动/指定订单核销、持久幂等、Ledger、真实资金流水、审计和账户不变量防线。预存充值将在后续阶段实现。

设计依据来自前端项目的 `docs/`：

- `frontend-contract-analysis.md`
- `api-contract.md`
- `accounting-rules.md`
- `database.md`
- `architecture.md`

开发说明见 `docs/development.md`。
