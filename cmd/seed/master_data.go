package main

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func seedMasterData(tx *gorm.DB, storeID, ownerID uuid.UUID) error {
	teams := []struct{ name, leader, phone string }{{"匠心施工队", "赵强", "13800138001"}, {"新城涂装队", "周明", "13800138002"}, {"安居油漆队", "陈建国", "13800138003"}}
	teamIDs := make([]uuid.UUID, len(teams))
	for i, v := range teams {
		teamIDs[i] = uuid.NewSHA1(storeID, []byte("team:"+v.name))
		if err := tx.Exec(`INSERT INTO teams(id,store_id,name,leader_name,phone,status,remark,version) VALUES(?,?,?,?,?,'ACTIVE','',1) ON CONFLICT(id) DO NOTHING`, teamIDs[i], storeID, v.name, v.leader, v.phone).Error; err != nil {
			return err
		}
	}
	workerNames := []string{"张建华", "李国强", "王师傅", "陈海波", "赵勇", "孙成", "周平", "黄文军", "吴建民", "郑师傅"}
	workerIDs := make([]uuid.UUID, len(workerNames))
	for i, name := range workerNames {
		workerIDs[i] = uuid.NewSHA1(storeID, []byte("worker:"+name))
		var team any
		if i < 7 {
			team = teamIDs[i%3]
		}
		if err := tx.Exec(`INSERT INTO workers(id,store_id,team_id,name,phone,status,remark,version,created_by) VALUES(?,?,?,?,?,'ACTIVE','',1,?) ON CONFLICT(id) DO NOTHING`, workerIDs[i], storeID, team, name, fmt.Sprintf("139001390%02d", i+1), ownerID).Error; err != nil {
			return err
		}
	}
	type projectSeed struct {
		name, address, manager, status string
		team                           int
		workers                        []int
	}
	projects := []projectSeed{{"滨江壹号 3 栋", "滨江路 88 号", "张建华", "ACTIVE", 0, []int{0, 1}}, {"万象城商铺改造", "中心大道万象城 B1", "王师傅", "ACTIVE", 1, []int{2, 4}}, {"金桂苑旧房翻新", "金桂苑 12-2-601", "陈海波", "PLANNING", 2, []int{3}}, {"云庭酒店翻新", "东湖路 16 号", "赵强", "COMPLETED", 0, []int{0, 5, 9}}, {"南苑办公楼", "科创园南苑 5 号楼", "周明", "CANCELLED", 1, []int{2}}}
	for _, p := range projects {
		id := uuid.NewSHA1(storeID, []byte("project:"+p.name))
		if err := tx.Exec(`INSERT INTO projects(id,store_id,team_id,name,address,manager_name,status,remark,version) VALUES(?,?,?,?,?,?,?,'',1) ON CONFLICT(id) DO NOTHING`, id, storeID, teamIDs[p.team], p.name, p.address, p.manager, p.status).Error; err != nil {
			return err
		}
		for _, wi := range p.workers {
			if err := tx.Exec(`INSERT INTO project_workers(store_id,project_id,worker_id) VALUES(?,?,?) ON CONFLICT DO NOTHING`, storeID, id, workerIDs[wi]).Error; err != nil {
				return err
			}
		}
	}
	categories := []string{"内墙漆", "外墙漆", "底漆", "辅材", "防水", "腻子"}
	brands := []string{"多乐士", "立邦", "三棵树", "华润漆", "东方雨虹"}
	materialIDs := make([]uuid.UUID, 24)
	for i := 0; i < 24; i++ {
		name := fmt.Sprintf("示范材料 %02d", i+1)
		materialIDs[i] = uuid.NewSHA1(storeID, []byte("material:"+name))
		price := fmt.Sprintf("%d.00", 48+i*17)
		cost := fmt.Sprintf("%d.00", 32+i*12)
		if err := tx.Exec(`INSERT INTO materials(id,store_id,name,category,brand,specification,unit,default_sale_price,cost_price,status,sales_count,version) VALUES(?,?,?,?,?,?,?,?::numeric,?::numeric,'ACTIVE',0,1) ON CONFLICT(id) DO NOTHING`, materialIDs[i], storeID, name, categories[i%len(categories)], brands[i%len(brands)], fmt.Sprintf("%dL", 5+i), "桶", price, cost).Error; err != nil {
			return err
		}
	}
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		id := uuid.NewSHA1(storeID, []byte(fmt.Sprintf("price:%d", i)))
		if err := tx.Exec(`INSERT INTO customer_prices(id,store_id,worker_id,material_id,price,effective_from,status,version,created_at,updated_at) VALUES(?,?,?,?,?::numeric,?,'ACTIVE',1,?,?) ON CONFLICT(id) DO NOTHING`, id, storeID, workerIDs[0], materialIDs[i], fmt.Sprintf("%d.00", 45+i*15), now, now, now).Error; err != nil {
			return err
		}
	}
	day := now.In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format("20060102")
	accountID := uuid.NewSHA1(storeID, []byte("prepaid-account:"+workerIDs[0].String()))
	insertAccount := tx.Exec(`INSERT INTO prepaid_accounts(id,store_id,worker_id,balance,version,created_at,updated_at) VALUES(?,?,?,2000,1,?,?) ON CONFLICT(store_id,worker_id) DO NOTHING`, accountID, storeID, workerIDs[0], now, now)
	if insertAccount.Error != nil {
		return insertAccount.Error
	}
	// Only create the opening balance and its paired financial record when this
	// seed owns the account creation. Existing business data must never be
	// overwritten or supplemented by a later seed run.
	if insertAccount.RowsAffected == 1 {
		if err := tx.Exec(`UPDATE workers SET prepaid_balance=2000,updated_at=? WHERE store_id=? AND id=?`, now, storeID, workerIDs[0]).Error; err != nil {
			return err
		}
		prepaidTransactionID := uuid.NewSHA1(storeID, []byte("seed-prepaid-deposit"))
		if err := tx.Exec(`INSERT INTO prepaid_transactions(id,store_id,worker_id,transaction_no,type,signed_amount,balance_before,balance_after,source_type,source_id,remark,occurred_at,created_by,created_at) VALUES(?,?,?,?,'DEPOSIT',2000,0,2000,'SEED',?,'开发平衡种子',?,?,?)`, prepaidTransactionID, storeID, workerIDs[0], "YC"+day+"0001", prepaidTransactionID, now, ownerID, now).Error; err != nil {
			return err
		}
		financialID := uuid.NewSHA1(storeID, []byte("seed-prepaid-finance"))
		if err := tx.Exec(`INSERT INTO financial_transactions(id,store_id,transaction_no,type,direction,amount,payment_method,worker_id,source_type,source_id,source_no,occurred_at,remark,created_by,created_at) VALUES(?,?,?,'PREPAID_DEPOSIT','INCOME',2000,'WECHAT',?,'PREPAID',?,?,?,'开发平衡种子',?,?)`, financialID, storeID, "LS"+day+"0001", workerIDs[0], prepaidTransactionID, "YC"+day+"0001", now, ownerID, now).Error; err != nil {
			return err
		}
		businessDate := now.In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format("2006-01-02")
		if err := tx.Exec(`INSERT INTO business_sequences(store_id,business_date,business_type,last_value) VALUES(?,?::date,'YC',1) ON CONFLICT(store_id,business_date,business_type) DO UPDATE SET last_value=GREATEST(business_sequences.last_value,1)`, storeID, businessDate).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO business_sequences(store_id,business_date,business_type,last_value) VALUES(?,?::date,'LS',1) ON CONFLICT(store_id,business_date,business_type) DO UPDATE SET last_value=GREATEST(business_sequences.last_value,1)`, storeID, businessDate).Error; err != nil {
			return err
		}
	}
	return nil
}
