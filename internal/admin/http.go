package admin

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"github.com/sqkrv/sharespences/internal/db"
)

// decToStr renders a nullable numeric for JSON (string keeps precision).
func decToStr(d *decimal.Decimal) *string {
	if d == nil {
		return nil
	}
	s := d.String()
	return &s
}

func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

// httpErr maps the service's error vocabulary onto responses. Everything a
// user (the operator) reads is Russian, like the app's API.
func httpErr(err error) error {
	switch {
	case errors.Is(err, ErrSeedManaged):
		return huma.Error409Conflict(ErrSeedManaged.Error())
	case errors.Is(err, pgx.ErrNoRows):
		return huma.Error404NotFound("не найдено")
	case isPgCode(err, "23505"):
		return huma.Error409Conflict("такая строка уже существует")
	case isPgCode(err, "23503"):
		return huma.Error409Conflict("строка используется другими записями")
	}
	return err
}

type pageParams struct {
	Query  string `query:"query" required:"false" doc:"подстрока названия (для MCC — или префикс кода)"`
	Limit  int32  `query:"limit" default:"50" minimum:"1" maximum:"500"`
	Offset int32  `query:"offset" default:"0" minimum:"0"`
}

// --- DTOs ---

type BankDTO struct {
	ID           int32   `json:"id"`
	Name         string  `json:"name"`
	ColorHex     *string `json:"color_hex,omitempty"`
	LogoFilename *string `json:"logo_filename,omitempty"`
}

type CanonicalDTO struct {
	ID      int64   `json:"id"`
	Slug    string  `json:"slug"`
	TitleRu string  `json:"title_ru"`
	Emoji   *string `json:"emoji,omitempty"`
}

type AliasDTO struct {
	BankID              int32  `json:"bank_id"`
	BankName            string `json:"bank_name"`
	RawTitle            string `json:"raw_title"`
	CanonicalCategoryID int64  `json:"canonical_category_id"`
	CanonicalSlug       string `json:"canonical_slug"`
	CanonicalTitle      string `json:"canonical_title"`
}

type ProgramDTO struct {
	ID                int64   `json:"id"`
	BankID            int32   `json:"bank_id"`
	BankName          string  `json:"bank_name"`
	Name              string  `json:"name"`
	PeriodType        string  `json:"period_type"`
	SelectionMode     string  `json:"selection_mode"`
	CurrencyKind      string  `json:"currency_kind"`
	PointsLabel       *string `json:"points_label,omitempty"`
	SelectionOpensDay *int32  `json:"selection_opens_day,omitempty"`
	Notes             *string `json:"notes,omitempty"`
}

type TierDTO struct {
	ID                 int64   `json:"id"`
	Name               string  `json:"name"`
	IsPaidSubscription bool    `json:"is_paid_subscription"`
	CapValue           *string `json:"cap_value,omitempty"`
	CapScope           string  `json:"cap_scope"`
	CapPerCategory     *string `json:"cap_per_category,omitempty"`
	MaxCategories      *int32  `json:"max_categories,omitempty"`
	Notes              *string `json:"notes,omitempty"`
}

type BankCategoryDTO struct {
	ID                  int64   `json:"id"`
	BankID              int32   `json:"bank_id"`
	BankName            string  `json:"bank_name"`
	Title               string  `json:"title"`
	CanonicalCategoryID *int64  `json:"canonical_category_id,omitempty"`
	CanonicalSlug       *string `json:"canonical_slug,omitempty"`
	CanonicalTitle      *string `json:"canonical_title,omitempty"`
	Kind                string  `json:"kind"`
	Emoji               *string `json:"emoji,omitempty"`
	IsCustom            bool    `json:"is_custom"`
	Active              bool    `json:"active"`
	MccCount            int64   `json:"mcc_count"`
	SeedManaged         bool    `json:"seed_managed" doc:"row itself is seed-refreshed (non-custom)"`
}

type MCCDTO struct {
	Code        int16   `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	SeedManaged bool    `json:"seed_managed"`
}

type MCCLinkDTO struct {
	MccCode int16   `json:"mcc_code"`
	MccName string  `json:"mcc_name"`
	Note    *string `json:"note,omitempty"`
}

type MCCLinksDTO struct {
	SeedManaged bool         `json:"seed_managed" doc:"always false since the ADR-0004 import took membership over from the seed"`
	Links       []MCCLinkDTO `json:"links"`
}

type MCCChangeDTO struct {
	ID             int64   `json:"id"`
	BankID         int32   `json:"bank_id"`
	BankName       string  `json:"bank_name"`
	BankCategoryID *int64  `json:"bank_category_id,omitempty"`
	CategoryTitle  string  `json:"category_title"`
	MccCode        *int16  `json:"mcc_code,omitempty"`
	Action         string  `json:"action"`
	NotedAt        string  `json:"noted_at"`
	Source         string  `json:"source"`
	Note           *string `json:"note,omitempty"`
}

type POSDTO struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	MerchantTitle   *string   `json:"merchant_title,omitempty"`
	MccCode         *int16    `json:"mcc_code,omitempty"`
	Type            *string   `json:"type,omitempty"`
	Address         *string   `json:"address,omitempty"`
	Confirmations   *int64    `json:"confirmations,omitempty"`
	CreatedAt       string    `json:"created_at"`
	LastConfirmedAt *string   `json:"last_confirmed_at,omitempty"`
}

type posBody struct {
	Name          string  `json:"name" minLength:"1"`
	MerchantTitle *string `json:"merchant_title,omitempty"`
	MccCode       *int16  `json:"mcc_code,omitempty" minimum:"0" maximum:"9999"`
	Type          *string `json:"type,omitempty" enum:"offline,online,app,other"`
	Address       *string `json:"address,omitempty"`
}

func posType(t *string) db.NullPointOfSaleType {
	if t == nil {
		return db.NullPointOfSaleType{}
	}
	return db.NullPointOfSaleType{PointOfSaleType: db.PointOfSaleType(*t), Valid: true}
}

// RegisterHTTP mounts the sidecar API (spec «API», docs/specs/admin.md).
// Read-only entities get list operations ONLY — write ops for seed-owned
// data are absent by design, not gated.
func RegisterHTTP(api huma.API, s *Service) {
	// --- dashboard ---

	huma.Register(api, huma.Operation{
		OperationID: "admin-dashboard", Method: http.MethodGet,
		Path: "/api/dashboard", Summary: "System dashboard", Tags: []string{"system"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body DashboardDTO }, error) {
		d, err := s.Dashboard(ctx)
		if err != nil {
			return nil, err
		}
		return &struct{ Body DashboardDTO }{dashboardDTO(d)}, nil
	})

	// --- read-only catalogs (seed-managed) ---

	huma.Register(api, huma.Operation{
		OperationID: "admin-bank-list", Method: http.MethodGet,
		Path: "/api/banks", Summary: "List banks (seed-managed)", Tags: []string{"catalog"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []BankDTO }, error) {
		rows, err := s.Q.ListBanks(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]BankDTO, len(rows))
		for i, b := range rows {
			out[i] = BankDTO{ID: b.ID, Name: b.Name, ColorHex: b.ColorHex, LogoFilename: b.LogoFilename}
		}
		return &struct{ Body []BankDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-canonical-list", Method: http.MethodGet,
		Path: "/api/canonical-categories", Summary: "List canonical categories (seed-managed)", Tags: []string{"catalog"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []CanonicalDTO }, error) {
		rows, err := s.Q.ListCanonicalCategories(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]CanonicalDTO, len(rows))
		for i, c := range rows {
			out[i] = CanonicalDTO{ID: c.ID, Slug: c.Slug, TitleRu: c.TitleRu, Emoji: c.Emoji}
		}
		return &struct{ Body []CanonicalDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-alias-list", Method: http.MethodGet,
		Path: "/api/aliases", Summary: "List bank category aliases (seed-managed)", Tags: []string{"catalog"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []AliasDTO }, error) {
		rows, err := s.Q.AdminListAliases(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]AliasDTO, len(rows))
		for i, a := range rows {
			out[i] = AliasDTO{
				BankID: a.BankID, BankName: a.BankName, RawTitle: a.RawTitle,
				CanonicalCategoryID: a.CanonicalCategoryID,
				CanonicalSlug:       a.CanonicalSlug, CanonicalTitle: a.CanonicalTitle,
			}
		}
		return &struct{ Body []AliasDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-program-list", Method: http.MethodGet,
		Path: "/api/programs", Summary: "List cashback programs (seed-managed)", Tags: []string{"catalog"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []ProgramDTO }, error) {
		rows, err := s.Q.ListPrograms(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]ProgramDTO, len(rows))
		for i, p := range rows {
			out[i] = ProgramDTO{
				ID: p.ID, BankID: p.BankID, BankName: p.BankName, Name: p.Name,
				PeriodType: string(p.PeriodType), SelectionMode: string(p.SelectionMode),
				CurrencyKind: string(p.CurrencyKind), PointsLabel: p.PointsLabel,
				SelectionOpensDay: p.SelectionOpensDay, Notes: p.Notes,
			}
		}
		return &struct{ Body []ProgramDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-tier-list", Method: http.MethodGet,
		Path: "/api/programs/{id}/tiers", Summary: "List tiers of a program (seed-managed)", Tags: []string{"catalog"},
	}, func(ctx context.Context, in *struct {
		ID int64 `path:"id"`
	}) (*struct{ Body []TierDTO }, error) {
		rows, err := s.Q.ListTiersForProgram(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		out := make([]TierDTO, len(rows))
		for i, t := range rows {
			out[i] = TierDTO{
				ID: t.ID, Name: t.Name, IsPaidSubscription: t.IsPaidSubscription,
				CapValue: decToStr(t.CapValue), CapScope: string(t.CapScope),
				CapPerCategory: decToStr(t.CapPerCategory), MaxCategories: t.MaxCategories, Notes: t.Notes,
			}
		}
		return &struct{ Body []TierDTO }{out}, nil
	})

	// --- bank categories (custom rows editable) ---

	huma.Register(api, huma.Operation{
		OperationID: "admin-bank-category-list", Method: http.MethodGet,
		Path: "/api/bank-categories", Summary: "List a bank's category catalog (incl. inactive)", Tags: []string{"catalog"},
	}, func(ctx context.Context, in *struct {
		BankID int32 `query:"bank_id" required:"false" doc:"0 или пусто — все банки"`
	}) (*struct{ Body []BankCategoryDTO }, error) {
		var bankFilter *int32
		if in.BankID != 0 {
			bankFilter = &in.BankID
		}
		rows, err := s.Q.AdminListBankCategories(ctx, bankFilter)
		if err != nil {
			return nil, err
		}
		out := make([]BankCategoryDTO, len(rows))
		for i, bc := range rows {
			out[i] = BankCategoryDTO{
				ID: bc.ID, BankID: bc.BankID, BankName: bc.BankName, Title: bc.Title,
				CanonicalCategoryID: bc.CanonicalCategoryID,
				CanonicalSlug:       bc.CanonicalSlug, CanonicalTitle: bc.CanonicalTitle,
				Kind: string(bc.Kind), Emoji: bc.Emoji,
				IsCustom: bc.IsCustom, Active: bc.Active, MccCount: bc.MccCount,
				SeedManaged: !bc.IsCustom,
			}
		}
		return &struct{ Body []BankCategoryDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-bank-category-update", Method: http.MethodPut,
		Path: "/api/bank-categories/{id}", Summary: "Edit a custom catalog row", Tags: []string{"catalog"},
	}, func(ctx context.Context, in *struct {
		ID   int64 `path:"id"`
		Body struct {
			Title               string  `json:"title" minLength:"1"`
			CanonicalCategoryID *int64  `json:"canonical_category_id,omitempty"`
			Kind                string  `json:"kind" enum:"regular,super,special"`
			Emoji               *string `json:"emoji,omitempty"`
			Active              bool    `json:"active"`
		}
	}) (*struct{ Body struct{ ID int64 } }, error) {
		if err := s.guardBankCategory(ctx, in.ID); err != nil {
			return nil, httpErr(err)
		}
		bc, err := s.Q.AdminUpdateCustomBankCategory(ctx, db.AdminUpdateCustomBankCategoryParams{
			ID: in.ID, Title: in.Body.Title,
			CanonicalCategoryID: in.Body.CanonicalCategoryID,
			Kind:                db.CashbackOfferKind(in.Body.Kind),
			Emoji:               in.Body.Emoji, Active: in.Body.Active,
		})
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body struct{ ID int64 } }{struct{ ID int64 }{bc.ID}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-bank-category-delete", Method: http.MethodDelete,
		Path: "/api/bank-categories/{id}", Summary: "Delete a custom catalog row", Tags: []string{"catalog"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID int64 `path:"id"`
	}) (*struct{}, error) {
		if err := s.guardBankCategory(ctx, in.ID); err != nil {
			return nil, httpErr(err)
		}
		if _, err := s.Q.AdminDeleteCustomBankCategory(ctx, in.ID); err != nil {
			return nil, httpErr(err)
		}
		return &struct{}{}, nil
	})

	// --- MCC dictionary ---

	huma.Register(api, huma.Operation{
		OperationID: "admin-mcc-list", Method: http.MethodGet,
		Path: "/api/mcc", Summary: "Search the MCC dictionary", Tags: []string{"mcc"},
	}, func(ctx context.Context, in *pageParams) (*struct {
		Body struct {
			Total int64    `json:"total"`
			Items []MCCDTO `json:"items"`
		}
	}, error) {
		rows, err := s.Q.AdminListMCC(ctx, db.AdminListMCCParams{
			Query: in.Query, MaxRows: in.Limit, Skip: in.Offset,
		})
		if err != nil {
			return nil, err
		}
		out := &struct {
			Body struct {
				Total int64    `json:"total"`
				Items []MCCDTO `json:"items"`
			}
		}{}
		out.Body.Items = make([]MCCDTO, len(rows))
		for i, m := range rows {
			out.Body.Total = m.Total
			out.Body.Items[i] = MCCDTO{
				Code: m.Code, Name: m.Name, Description: m.Description,
				SeedManaged: s.SeededMCC[m.Code],
			}
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-mcc-create", Method: http.MethodPost,
		Path: "/api/mcc", Summary: "Add a dictionary code the seed does not carry", Tags: []string{"mcc"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Body struct {
			Code        int16   `json:"code" minimum:"0" maximum:"9999"`
			Name        string  `json:"name" minLength:"1"`
			Description *string `json:"description,omitempty"`
		}
	}) (*struct{ Body MCCDTO }, error) {
		if err := s.guardMCC(in.Body.Code); err != nil {
			return nil, httpErr(err)
		}
		m, err := s.Q.AdminCreateMCC(ctx, db.AdminCreateMCCParams{
			Code: in.Body.Code, Name: in.Body.Name, Description: in.Body.Description,
		})
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body MCCDTO }{MCCDTO{Code: m.Code, Name: m.Name, Description: m.Description}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-mcc-update", Method: http.MethodPut,
		Path: "/api/mcc/{code}", Summary: "Edit a non-seeded dictionary code", Tags: []string{"mcc"},
	}, func(ctx context.Context, in *struct {
		Code int16 `path:"code"`
		Body struct {
			Name        string  `json:"name" minLength:"1"`
			Description *string `json:"description,omitempty"`
		}
	}) (*struct{ Body MCCDTO }, error) {
		if err := s.guardMCC(in.Code); err != nil {
			return nil, httpErr(err)
		}
		m, err := s.Q.AdminUpdateMCC(ctx, db.AdminUpdateMCCParams{
			Code: in.Code, Name: in.Body.Name, Description: in.Body.Description,
		})
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body MCCDTO }{MCCDTO{Code: m.Code, Name: m.Name, Description: m.Description}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-mcc-delete", Method: http.MethodDelete,
		Path: "/api/mcc/{code}", Summary: "Delete a non-seeded dictionary code", Tags: []string{"mcc"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		Code int16 `path:"code"`
	}) (*struct{}, error) {
		if err := s.guardMCC(in.Code); err != nil {
			return nil, httpErr(err)
		}
		n, err := s.Q.AdminDeleteMCC(ctx, in.Code)
		if err != nil {
			return nil, httpErr(err)
		}
		if n == 0 {
			return nil, huma.Error404NotFound("не найдено")
		}
		return &struct{}{}, nil
	})

	// --- category ↔ MCC links ---

	huma.Register(api, huma.Operation{
		OperationID: "admin-mcc-link-list", Method: http.MethodGet,
		Path: "/api/bank-categories/{id}/mcc", Summary: "MCC links of a catalog row", Tags: []string{"mcc"},
	}, func(ctx context.Context, in *struct {
		ID int64 `path:"id"`
	}) (*struct{ Body MCCLinksDTO }, error) {
		if _, err := s.Q.AdminGetBankCategoryWithBank(ctx, in.ID); err != nil {
			return nil, httpErr(err)
		}
		rows, err := s.Q.AdminListBankCategoryMCC(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		out := MCCLinksDTO{
			SeedManaged: false, // membership is import-managed since ADR-0004; edits stand until a snapshot re-lands (journaled)
			Links:       make([]MCCLinkDTO, len(rows)),
		}
		for i, l := range rows {
			out.Links[i] = MCCLinkDTO{MccCode: l.MccCode, MccName: l.MccName, Note: l.Note}
		}
		return &struct{ Body MCCLinksDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-mcc-link-upsert", Method: http.MethodPut,
		Path: "/api/bank-categories/{id}/mcc/{code}", Summary: "Add/update an MCC link", Tags: []string{"mcc"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID   int64 `path:"id"`
		Code int16 `path:"code"`
		Body struct {
			Note *string `json:"note,omitempty"`
		}
	}) (*struct{}, error) {
		if err := s.requireBankCategory(ctx, in.ID); err != nil {
			return nil, httpErr(err)
		}
		if err := s.Q.AdminUpsertBankCategoryMCC(ctx, db.AdminUpsertBankCategoryMCCParams{
			BankCategoryID: in.ID, MccCode: in.Code, Note: in.Body.Note,
		}); err != nil {
			if isPgCode(err, "23503") {
				return nil, huma.Error422UnprocessableEntity("код MCC отсутствует в справочнике — сначала добавь его")
			}
			return nil, httpErr(err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-mcc-link-delete", Method: http.MethodDelete,
		Path: "/api/bank-categories/{id}/mcc/{code}", Summary: "Remove an MCC link", Tags: []string{"mcc"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID   int64 `path:"id"`
		Code int16 `path:"code"`
	}) (*struct{}, error) {
		if err := s.requireBankCategory(ctx, in.ID); err != nil {
			return nil, httpErr(err)
		}
		n, err := s.Q.AdminDeleteBankCategoryMCC(ctx, db.AdminDeleteBankCategoryMCCParams{
			BankCategoryID: in.ID, MccCode: in.Code,
		})
		if err != nil {
			return nil, httpErr(err)
		}
		if n == 0 {
			return nil, huma.Error404NotFound("не найдено")
		}
		return &struct{}{}, nil
	})

	// --- mcc_change journal ---

	huma.Register(api, huma.Operation{
		OperationID: "admin-mcc-change-list", Method: http.MethodGet,
		Path: "/api/mcc-changes", Summary: "MCC change journal", Tags: []string{"mcc"},
	}, func(ctx context.Context, in *struct {
		Limit  int32 `query:"limit" default:"50" minimum:"1" maximum:"500"`
		Offset int32 `query:"offset" default:"0" minimum:"0"`
	}) (*struct {
		Body struct {
			Total int64          `json:"total"`
			Items []MCCChangeDTO `json:"items"`
		}
	}, error) {
		rows, err := s.Q.AdminListMCCChanges(ctx, db.AdminListMCCChangesParams{
			MaxRows: in.Limit, Skip: in.Offset,
		})
		if err != nil {
			return nil, err
		}
		out := &struct {
			Body struct {
				Total int64          `json:"total"`
				Items []MCCChangeDTO `json:"items"`
			}
		}{}
		out.Body.Items = make([]MCCChangeDTO, len(rows))
		for i, c := range rows {
			out.Body.Total = c.Total
			out.Body.Items[i] = MCCChangeDTO{
				ID: c.ID, BankID: c.BankID, BankName: c.BankName,
				BankCategoryID: c.BankCategoryID, CategoryTitle: c.CategoryTitle,
				MccCode: c.MccCode, Action: string(c.Action),
				NotedAt: c.NotedAt.Format("2006-01-02 15:04"), Source: c.Source, Note: c.Note,
			}
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-mcc-change-create", Method: http.MethodPost,
		Path: "/api/mcc-changes", Summary: "Record a hand-observed MCC change", Tags: []string{"mcc"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Body struct {
			BankID         int32   `json:"bank_id"`
			BankCategoryID *int64  `json:"bank_category_id,omitempty"`
			CategoryTitle  string  `json:"category_title" minLength:"1"`
			MccCode        *int16  `json:"mcc_code,omitempty" minimum:"0" maximum:"9999"`
			Action         string  `json:"action" enum:"added,removed,category_added,category_removed"`
			Source         string  `json:"source,omitempty" doc:"empty → «manual (admin)»"`
			Note           *string `json:"note,omitempty"`
		}
	}) (*struct{ Body struct{ ID int64 } }, error) {
		source := in.Body.Source
		if source == "" {
			source = "manual (admin)"
		}
		id, err := s.Q.AdminCreateMCCChange(ctx, db.AdminCreateMCCChangeParams{
			BankID: in.Body.BankID, BankCategoryID: in.Body.BankCategoryID,
			CategoryTitle: in.Body.CategoryTitle, MccCode: in.Body.MccCode,
			Action: db.MccChangeAction(in.Body.Action), Source: source, Note: in.Body.Note,
		})
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body struct{ ID int64 } }{struct{ ID int64 }{id}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-mcc-change-delete", Method: http.MethodDelete,
		Path: "/api/mcc-changes/{id}", Summary: "Delete a journal entry (typo repair)", Tags: []string{"mcc"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID int64 `path:"id"`
	}) (*struct{}, error) {
		n, err := s.Q.AdminDeleteMCCChange(ctx, in.ID)
		if err != nil {
			return nil, httpErr(err)
		}
		if n == 0 {
			return nil, huma.Error404NotFound("не найдено")
		}
		return &struct{}{}, nil
	})

	// --- points of sale ---

	huma.Register(api, huma.Operation{
		OperationID: "admin-pos-list", Method: http.MethodGet,
		Path: "/api/pos", Summary: "Search points of sale", Tags: []string{"pos"},
	}, func(ctx context.Context, in *pageParams) (*struct {
		Body struct {
			Total int64    `json:"total"`
			Items []POSDTO `json:"items"`
		}
	}, error) {
		rows, err := s.Q.AdminSearchPOS(ctx, db.AdminSearchPOSParams{
			Query: in.Query, MaxRows: in.Limit, Skip: in.Offset,
		})
		if err != nil {
			return nil, err
		}
		out := &struct {
			Body struct {
				Total int64    `json:"total"`
				Items []POSDTO `json:"items"`
			}
		}{}
		out.Body.Items = make([]POSDTO, len(rows))
		for i, p := range rows {
			out.Body.Total = p.Total
			d := POSDTO{
				ID: p.ID, Name: p.Name, MerchantTitle: p.MerchantTitle,
				MccCode: p.MccCode, Address: p.Address, Confirmations: p.Confirmations,
				CreatedAt: p.CreatedAt.Format("2006-01-02"),
			}
			if p.Type.Valid {
				t := string(p.Type.PointOfSaleType)
				d.Type = &t
			}
			if p.LastConfirmedAt != nil {
				t := p.LastConfirmedAt.Format("2006-01-02")
				d.LastConfirmedAt = &t
			}
			out.Body.Items[i] = d
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-pos-create", Method: http.MethodPost,
		Path: "/api/pos", Summary: "Add a point of sale", Tags: []string{"pos"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Body posBody
	}) (*struct{ Body struct{ ID uuid.UUID } }, error) {
		id, err := s.Q.AdminCreatePOS(ctx, db.AdminCreatePOSParams{
			Name: in.Body.Name, MerchantTitle: in.Body.MerchantTitle,
			MccCode: in.Body.MccCode, Type: posType(in.Body.Type), Address: in.Body.Address,
		})
		if err != nil {
			if isPgCode(err, "23503") {
				return nil, huma.Error422UnprocessableEntity("код MCC отсутствует в справочнике — сначала добавь его")
			}
			return nil, httpErr(err)
		}
		return &struct{ Body struct{ ID uuid.UUID } }{struct{ ID uuid.UUID }{id}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-pos-update", Method: http.MethodPut,
		Path: "/api/pos/{id}", Summary: "Edit a point of sale", Tags: []string{"pos"},
	}, func(ctx context.Context, in *struct {
		ID   uuid.UUID `path:"id"`
		Body posBody
	}) (*struct{ Body struct{ ID uuid.UUID } }, error) {
		id, err := s.Q.AdminUpdatePOS(ctx, db.AdminUpdatePOSParams{
			ID: in.ID, Name: in.Body.Name, MerchantTitle: in.Body.MerchantTitle,
			MccCode: in.Body.MccCode, Type: posType(in.Body.Type), Address: in.Body.Address,
		})
		if err != nil {
			if isPgCode(err, "23503") {
				return nil, huma.Error422UnprocessableEntity("код MCC отсутствует в справочнике — сначала добавь его")
			}
			return nil, httpErr(err)
		}
		return &struct{ Body struct{ ID uuid.UUID } }{struct{ ID uuid.UUID }{id}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "admin-pos-delete", Method: http.MethodDelete,
		Path: "/api/pos/{id}", Summary: "Delete a point of sale", Tags: []string{"pos"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID uuid.UUID `path:"id"`
	}) (*struct{}, error) {
		n, err := s.Q.AdminDeletePOS(ctx, in.ID)
		if err != nil {
			return nil, httpErr(err)
		}
		if n == 0 {
			return nil, huma.Error404NotFound("не найдено")
		}
		return &struct{}{}, nil
	})
}

// --- dashboard DTO ---

type MigrationDTO struct {
	Version   int64   `json:"version"`
	Source    string  `json:"source"`
	AppliedAt *string `json:"applied_at,omitempty" doc:"null = pending"`
}

type TableCountDTO struct {
	Table string `json:"table"`
	Rows  int64  `json:"rows"`
}

type CountsDTO struct {
	Banks                int64 `json:"banks"`
	CanonicalCategories  int64 `json:"canonical_categories"`
	BankCategories       int64 `json:"bank_categories"`
	CustomBankCategories int64 `json:"custom_bank_categories"`
	Aliases              int64 `json:"aliases"`
	Programs             int64 `json:"programs"`
	Tiers                int64 `json:"tiers"`
	MccCodes             int64 `json:"mcc_codes"`
	MccLinks             int64 `json:"mcc_links"`
	MccChanges           int64 `json:"mcc_changes"`
	PointsOfSale         int64 `json:"points_of_sale"`
	Users                int64 `json:"users"`
	Attachments          int64 `json:"attachments"`
}

type DashboardDTO struct {
	DBSizeBytes int64           `json:"db_size_bytes"`
	Counts      CountsDTO       `json:"counts"`
	Tables      []TableCountDTO `json:"tables" doc:"pg_stat estimates, incl. legacy schema-only tables"`
	Migrations  []MigrationDTO  `json:"migrations"`
}

func dashboardDTO(d DashboardData) DashboardDTO {
	out := DashboardDTO{
		DBSizeBytes: d.DBSizeBytes,
		Counts: CountsDTO{
			Banks: d.Counts.Banks, CanonicalCategories: d.Counts.CanonicalCategories,
			BankCategories: d.Counts.BankCategories, CustomBankCategories: d.Counts.CustomBankCategories,
			Aliases: d.Counts.Aliases, Programs: d.Counts.Programs, Tiers: d.Counts.Tiers,
			MccCodes: d.Counts.MccCodes, MccLinks: d.Counts.MccLinks, MccChanges: d.Counts.MccChanges,
			PointsOfSale: d.Counts.PointsOfSale, Users: d.Counts.Users, Attachments: d.Counts.Attachments,
		},
		Tables:     make([]TableCountDTO, len(d.Tables)),
		Migrations: make([]MigrationDTO, len(d.Migrations)),
	}
	for i, t := range d.Tables {
		out.Tables[i] = TableCountDTO(t)
	}
	for i, m := range d.Migrations {
		out.Migrations[i] = MigrationDTO{Version: m.Version, Source: m.Source}
		if m.AppliedAt != nil {
			s := m.AppliedAt.Format("2006-01-02 15:04")
			out.Migrations[i].AppliedAt = &s
		}
	}
	return out
}
