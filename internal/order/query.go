package order

import (
	"cloud-ledger-backend/internal/platform/apperror"
	"cloud-ledger-backend/internal/platform/pagination"
	"context"
	"github.com/google/uuid"
)

type queryRow struct {
	Model
	OperatorName string
}

func (s *Service) List(ctx context.Context, store uuid.UUID, q map[string]string, p pagination.Params) ([]Response, int64, error) {
	db := s.db.WithContext(ctx).Table("orders o").Joins("JOIN stores s ON s.id=o.store_id").Where("o.store_id=?", store)
	if v := q["search"]; v != "" {
		like := "%" + v + "%"
		db = db.Where("o.order_no ILIKE ? OR o.worker_name_snapshot ILIKE ? OR o.project_name_snapshot ILIKE ?", like, like, like)
	}
	if v := q["workerId"]; v != "" {
		db = db.Where("o.worker_id=?", v)
	}
	if v := q["status"]; v != "" {
		db = db.Where("o.status=?", v)
	}
	if v := q["from"]; v != "" {
		db = db.Where("(o.occurred_at AT TIME ZONE s.timezone)::date>=?::date", v)
	}
	if v := q["to"]; v != "" {
		db = db.Where("(o.occurred_at AT TIME ZONE s.timezone)::date<=?::date", v)
	}
	var total int64
	if e := db.Count(&total).Error; e != nil {
		return nil, 0, e
	}
	var rows []queryRow
	if e := db.Select("o.*,u.name AS operator_name").Joins("JOIN users u ON u.id=o.created_by AND u.store_id=o.store_id").Order("o.occurred_at DESC,o.created_at DESC").Offset(p.Offset()).Limit(p.PageSize).Scan(&rows).Error; e != nil {
		return nil, 0, e
	}
	items, e := s.responses(ctx, store, rows)
	return items, total, e
}
func (s *Service) Get(ctx context.Context, store, id uuid.UUID) (Response, error) {
	var row queryRow
	if e := s.db.WithContext(ctx).Table("orders o").Select("o.*,u.name AS operator_name").Joins("JOIN users u ON u.id=o.created_by AND u.store_id=o.store_id").Where("o.store_id=? AND o.id=?", store, id).Scan(&row).Error; e != nil {
		return Response{}, e
	}
	if row.ID == uuid.Nil {
		return Response{}, apperror.New(404, "ORDER_NOT_FOUND", "用料单不存在。")
	}
	items, e := s.responses(ctx, store, []queryRow{row})
	if e != nil {
		return Response{}, e
	}
	return items[0], nil
}
func (s *Service) responses(ctx context.Context, store uuid.UUID, rows []queryRow) ([]Response, error) {
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	var all []ItemModel
	if len(ids) > 0 {
		if e := s.db.WithContext(ctx).Where("store_id=? AND order_id IN ?", store, ids).Order("created_at,id").Find(&all).Error; e != nil {
			return nil, e
		}
	}
	by := map[uuid.UUID][]ItemResponse{}
	for _, i := range all {
		by[i.OrderID] = append(by[i.OrderID], ItemResponse{MaterialID: i.MaterialID.String(), MaterialName: i.MaterialNameSnapshot, Specification: i.SpecificationSnapshot, Unit: i.UnitSnapshot, Quantity: i.Quantity.String(), UnitPrice: i.UnitPrice.StringFixed(2), Discount: i.DiscountAmount.StringFixed(2), Subtotal: i.Subtotal.StringFixed(2)})
	}
	out := make([]Response, len(rows))
	for i, r := range rows {
		out[i] = responseFrom(r.Model, by[r.ID], r.OperatorName)
		if out[i].Items == nil {
			out[i].Items = []ItemResponse{}
		}
	}
	return out, nil
}
