package project

import (
	"cloud-ledger-backend/internal/auth"
	"cloud-ledger-backend/internal/platform/apperror"
	"cloud-ledger-backend/internal/platform/pagination"
	platformrequest "cloud-ledger-backend/internal/platform/request"
	"cloud-ledger-backend/internal/platform/response"
	"context"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"net/http"
	"strings"
	"time"
)

type Model struct {
	ID                         uuid.UUID
	StoreID                    uuid.UUID
	TeamID                     *uuid.UUID
	Name, Address, ManagerName string
	StartDate, EndDate         *time.Time
	Status, Remark             string
	MaterialTotal              decimal.Decimal
	Version                    int64
	CreatedAt, UpdatedAt       time.Time
}

func (Model) TableName() string { return "projects" }

type ProjectWorker struct {
	StoreID, ProjectID, WorkerID uuid.UUID
	CreatedAt                    time.Time
}

func (ProjectWorker) TableName() string { return "project_workers" }

type Input struct {
	Name      string   `json:"name"`
	Address   string   `json:"address"`
	Manager   string   `json:"manager"`
	WorkerIDs []string `json:"workerIds"`
	TeamID    *string  `json:"teamId"`
	StartDate *string  `json:"startDate"`
	EndDate   *string  `json:"endDate"`
	Status    string   `json:"status"`
	Note      string   `json:"note"`
	Version   *int64   `json:"version,omitempty"`
}
type Response struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Address       string   `json:"address"`
	Manager       string   `json:"manager"`
	WorkerIDs     []string `json:"workerIds"`
	WorkerNames   []string `json:"workerNames"`
	TeamID        *string  `json:"teamId"`
	TeamName      *string  `json:"teamName"`
	StartDate     *string  `json:"startDate"`
	EndDate       *string  `json:"endDate"`
	Status        string   `json:"status"`
	Note          string   `json:"note"`
	MaterialTotal string   `json:"materialTotal"`
	Version       int64    `json:"version"`
}
type row struct {
	Model
	TeamName *string
}
type Service struct{ db *gorm.DB }

func NewService(db *gorm.DB) *Service { return &Service{db: db} }
func (s *Service) base(ctx context.Context, store uuid.UUID) *gorm.DB {
	return s.db.WithContext(ctx).Table("projects p").Select("p.*,t.name AS team_name").Joins("LEFT JOIN teams t ON t.id=p.team_id AND t.store_id=p.store_id").Where("p.store_id=?", store)
}
func (s *Service) decorate(ctx context.Context, store uuid.UUID, rows []row) ([]Response, error) {
	projectIDs := make([]uuid.UUID, len(rows))
	for i, item := range rows {
		projectIDs[i] = item.ID
	}
	type memberRow struct {
		ProjectID uuid.UUID
		ID        uuid.UUID
		Name      string
	}
	var memberRows []memberRow
	if len(projectIDs) > 0 {
		if err := s.db.WithContext(ctx).Table("workers w").Select("pw.project_id,w.id,w.name").Joins("JOIN project_workers pw ON pw.worker_id=w.id AND pw.store_id=w.store_id").Where("pw.store_id=? AND pw.project_id IN ?", store, projectIDs).Order("pw.created_at,w.id").Scan(&memberRows).Error; err != nil {
			return nil, err
		}
	}
	membersByProject := make(map[uuid.UUID][]struct {
		ID   uuid.UUID
		Name string
	}, len(rows))
	for _, member := range memberRows {
		membersByProject[member.ProjectID] = append(membersByProject[member.ProjectID], struct {
			ID   uuid.UUID
			Name string
		}{member.ID, member.Name})
	}
	result := make([]Response, len(rows))
	for i, r := range rows {
		result[i] = mapResponse(r, membersByProject[r.ID])
	}
	return result, nil
}
func (s *Service) List(ctx context.Context, store uuid.UUID, q map[string]string, p pagination.Params) ([]Response, int64, error) {
	db := s.db.WithContext(ctx).Table("projects p").Where("p.store_id=?", store)
	if v := q["search"]; v != "" {
		like := "%" + v + "%"
		db = db.Where("p.name ILIKE ? OR p.address ILIKE ? OR p.manager_name ILIKE ?", like, like, like)
	}
	if v := q["status"]; v != "" {
		db = db.Where("p.status=?", v)
	}
	if v := q["teamId"]; v != "" {
		db = db.Where("p.team_id=?", v)
	}
	if v := q["workerId"]; v != "" {
		db = db.Where("EXISTS (SELECT 1 FROM project_workers pw WHERE pw.project_id=p.id AND pw.store_id=p.store_id AND pw.worker_id=?)", v)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []row
	if err := db.Select("p.*,t.name AS team_name").Joins("LEFT JOIN teams t ON t.id=p.team_id AND t.store_id=p.store_id").Order("p.created_at DESC").Offset(p.Offset()).Limit(p.PageSize).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	items, err := s.decorate(ctx, store, rows)
	return items, total, err
}
func (s *Service) Get(ctx context.Context, store, id uuid.UUID) (Response, error) {
	var r row
	if err := s.base(ctx, store).Where("p.id=?", id).Scan(&r).Error; err != nil {
		return Response{}, err
	}
	if r.ID == uuid.Nil {
		return Response{}, apperror.New(404, "PROJECT_NOT_FOUND", "项目不存在。")
	}
	items, err := s.decorate(ctx, store, []row{r})
	if err != nil {
		return Response{}, err
	}
	return items[0], nil
}
func (s *Service) refs(tx *gorm.DB, store uuid.UUID, input Input) (*uuid.UUID, []uuid.UUID, error) {
	var teamID *uuid.UUID
	if input.TeamID != nil && *input.TeamID != "" {
		id, err := uuid.Parse(*input.TeamID)
		if err != nil {
			return nil, nil, apperror.Validation(map[string][]string{"teamId": {"施工队无效"}})
		}
		var n int64
		tx.Table("teams").Where("store_id=? AND id=?", store, id).Count(&n)
		if n == 0 {
			return nil, nil, apperror.New(404, "TEAM_NOT_FOUND", "施工队不存在。")
		}
		teamID = &id
	}
	ids := make([]uuid.UUID, 0, len(input.WorkerIDs))
	seen := map[uuid.UUID]bool{}
	for _, raw := range input.WorkerIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, nil, apperror.Validation(map[string][]string{"workerIds": {"油漆工无效"}})
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	var count int64
	tx.Table("workers").Where("store_id=? AND id IN ?", store, ids).Count(&count)
	if count != int64(len(ids)) {
		return nil, nil, apperror.New(404, "WORKER_NOT_FOUND", "油漆工不存在。")
	}
	return teamID, ids, nil
}
func (s *Service) save(ctx context.Context, store, id uuid.UUID, input Input, create bool) (Response, error) {
	start, end, err := validate(input)
	if err != nil {
		return Response{}, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		teamID, workerIDs, err := s.refs(tx, store, input)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if create {
			status := input.Status
			if status == "" {
				status = "PLANNING"
			}
			m := Model{ID: id, StoreID: store, TeamID: teamID, Name: strings.TrimSpace(input.Name), Address: strings.TrimSpace(input.Address), ManagerName: strings.TrimSpace(input.Manager), StartDate: start, EndDate: end, Status: status, Remark: strings.TrimSpace(input.Note), MaterialTotal: decimal.Zero, Version: 1, CreatedAt: now, UpdatedAt: now}
			if err = tx.Create(&m).Error; err != nil {
				return err
			}
		} else {
			if input.Version == nil {
				return apperror.New(409, "VERSION_CONFLICT", "资料已被其他人修改，请刷新后重试。")
			}
			res := tx.Model(&Model{}).Where("store_id=? AND id=? AND version=?", store, id, *input.Version).Updates(map[string]any{"team_id": teamID, "name": strings.TrimSpace(input.Name), "address": strings.TrimSpace(input.Address), "manager_name": strings.TrimSpace(input.Manager), "start_date": start, "end_date": end, "status": input.Status, "remark": strings.TrimSpace(input.Note), "version": gorm.Expr("version+1"), "updated_at": now})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				var count int64
				if err = tx.Model(&Model{}).Where("store_id=? AND id=?", store, id).Count(&count).Error; err != nil {
					return err
				}
				if count == 0 {
					return apperror.New(404, "PROJECT_NOT_FOUND", "项目不存在。")
				}
				return apperror.New(409, "VERSION_CONFLICT", "资料已被其他人修改，请刷新后重试。")
			}
			if err = tx.Where("store_id=? AND project_id=?", store, id).Delete(&ProjectWorker{}).Error; err != nil {
				return err
			}
		}
		links := make([]ProjectWorker, len(workerIDs))
		for i, w := range workerIDs {
			links[i] = ProjectWorker{StoreID: store, ProjectID: id, WorkerID: w, CreatedAt: now}
		}
		if len(links) > 0 {
			return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&links).Error
		}
		return nil
	})
	if err != nil {
		return Response{}, err
	}
	return s.Get(ctx, store, id)
}
func (s *Service) Create(ctx context.Context, store uuid.UUID, input Input) (Response, error) {
	return s.save(ctx, store, uuid.New(), input, true)
}
func (s *Service) Update(ctx context.Context, store, id uuid.UUID, input Input) (Response, error) {
	return s.save(ctx, store, id, input, false)
}
func parseDate(v *string) (*time.Time, error) {
	if v == nil || *v == "" {
		return nil, nil
	}
	d, e := time.Parse("2006-01-02", *v)
	return &d, e
}
func validate(i Input) (*time.Time, *time.Time, error) {
	if strings.TrimSpace(i.Name) == "" || len([]rune(i.Name)) > 80 || strings.TrimSpace(i.Address) == "" || len([]rune(i.Address)) > 120 || strings.TrimSpace(i.Manager) == "" || len([]rune(i.Manager)) > 30 || len(i.WorkerIDs) == 0 || len([]rune(i.Note)) > 200 {
		return nil, nil, apperror.Validation(map[string][]string{"form": {"请检查项目资料。"}})
	}
	start, e := parseDate(i.StartDate)
	if e != nil {
		return nil, nil, apperror.Validation(map[string][]string{"startDate": {"日期格式不正确"}})
	}
	end, e := parseDate(i.EndDate)
	if e != nil {
		return nil, nil, apperror.Validation(map[string][]string{"endDate": {"日期格式不正确"}})
	}
	if start != nil && end != nil && end.Before(*start) {
		return nil, nil, apperror.Validation(map[string][]string{"endDate": {"结束日期不能早于开始日期"}})
	}
	switch i.Status {
	case "", "PLANNING", "ACTIVE", "COMPLETED", "CANCELLED":
	default:
		return nil, nil, apperror.Validation(map[string][]string{"status": {"状态无效"}})
	}
	return start, end, nil
}
func mapResponse(r row, m []struct {
	ID   uuid.UUID
	Name string
}) Response {
	ids := make([]string, len(m))
	names := make([]string, len(m))
	for i, v := range m {
		ids[i] = v.ID.String()
		names[i] = v.Name
	}
	var teamID *string
	if r.TeamID != nil {
		v := r.TeamID.String()
		teamID = &v
	}
	date := func(v *time.Time) *string {
		if v == nil {
			return nil
		}
		s := v.Format("2006-01-02")
		return &s
	}
	return Response{ID: r.ID.String(), Name: r.Name, Address: r.Address, Manager: r.ManagerName, WorkerIDs: ids, WorkerNames: names, TeamID: teamID, TeamName: r.TeamName, StartDate: date(r.StartDate), EndDate: date(r.EndDate), Status: r.Status, Note: r.Remark, MaterialTotal: r.MaterialTotal.StringFixed(2), Version: r.Version}
}

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }
func (h *Handler) List(c *gin.Context) {
	store, e := platformrequest.UUID(c, auth.ContextStoreID)
	if e != nil {
		response.WriteError(c, e)
		return
	}
	q := map[string]string{}
	for _, k := range []string{"search", "status", "teamId", "workerId"} {
		q[k] = c.Query(k)
	}
	p := pagination.Parse(c.Request.URL.Query())
	items, total, e := h.service.List(c.Request.Context(), store, q, p)
	if e != nil {
		response.WriteError(c, e)
		return
	}
	c.JSON(200, pagination.Page[Response]{Items: items, Page: p.Page, PageSize: p.PageSize, Total: total})
}
func (h *Handler) Get(c *gin.Context) {
	s, id, e := ids(c)
	if e != nil {
		response.WriteError(c, e)
		return
	}
	v, e := h.service.Get(c.Request.Context(), s, id)
	if e != nil {
		response.WriteError(c, e)
		return
	}
	c.JSON(200, v)
}
func (h *Handler) write(c *gin.Context, create bool) {
	s, e := platformrequest.UUID(c, auth.ContextStoreID)
	if e != nil {
		response.WriteError(c, e)
		return
	}
	var in Input
	if c.ShouldBindJSON(&in) != nil {
		response.WriteError(c, apperror.Validation(map[string][]string{"body": {"请求格式不正确"}}))
		return
	}
	var v Response
	if create {
		v, e = h.service.Create(c.Request.Context(), s, in)
	} else {
		var id uuid.UUID
		id, e = platformrequest.ParamUUID(c, "id")
		if e == nil {
			v, e = h.service.Update(c.Request.Context(), s, id, in)
		}
	}
	if e != nil {
		response.WriteError(c, e)
		return
	}
	status := 200
	if create {
		status = http.StatusCreated
	}
	c.JSON(status, v)
}
func (h *Handler) Create(c *gin.Context) { h.write(c, true) }
func (h *Handler) Update(c *gin.Context) { h.write(c, false) }
func ids(c *gin.Context) (uuid.UUID, uuid.UUID, error) {
	s, e := platformrequest.UUID(c, auth.ContextStoreID)
	if e != nil {
		return uuid.Nil, uuid.Nil, e
	}
	id, e := platformrequest.ParamUUID(c, "id")
	return s, id, e
}
func RegisterRoutes(api *gin.RouterGroup, h *Handler, m auth.Middleware) {
	g := api.Group("/projects", m.Authenticate())
	g.GET("", auth.Require("workers:read"), h.List)
	g.GET("/:id", auth.Require("workers:read"), h.Get)
	g.POST("", auth.Require("workers:write"), h.Create)
	g.PATCH("/:id", auth.Require("workers:write"), h.Update)
}
