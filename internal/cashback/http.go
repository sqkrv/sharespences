package cashback

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/sqkrv/sharespences/internal/auth"
	"github.com/sqkrv/sharespences/internal/db"
)

// Decimal values travel as strings in the JSON API («5», «7.5», «1500») —
// exact, no float rounding, and huma schemas stay primitive.

func decToStr(d *decimal.Decimal) *string {
	if d == nil {
		return nil
	}
	s := d.String()
	return &s
}

func strToDec(s *string, field string) (*decimal.Decimal, error) {
	if s == nil {
		return nil, nil
	}
	d, err := decimal.NewFromString(*s)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(fmt.Sprintf("%s: not a decimal number: %q", field, *s))
	}
	return &d, nil
}

func parseDate(s string, field string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, huma.Error422UnprocessableEntity(fmt.Sprintf("%s: want YYYY-MM-DD, got %q", field, s))
	}
	return t, nil
}

func httpErr(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return huma.Error404NotFound("not found")
	case errors.Is(err, ErrSlotsExhausted),
		errors.Is(err, ErrAlreadySelected),
		errors.Is(err, ErrPeriodOverlap):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, ErrBankCategoryExists):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, ErrOutsidePeriod), errors.Is(err, ErrInvalidPeriod),
		errors.Is(err, ErrBankCategoryWrongBank):
		return huma.Error422UnprocessableEntity(err.Error())
	}
	return err
}

type ProgramDTO struct {
	ID                int64   `json:"id"`
	BankID            int32   `json:"bank_id"`
	BankName          string  `json:"bank_name,omitempty"`
	Name              string  `json:"name"`
	PeriodType        string  `json:"period_type"`
	SelectionMode     string  `json:"selection_mode"`
	CurrencyKind      string  `json:"currency_kind"`
	PointsLabel       *string `json:"points_label,omitempty"`
	SelectionOpensDay *int32  `json:"selection_opens_day,omitempty"`
	Notes             *string `json:"notes,omitempty"`
}

func programDTO(p db.CashbackProgram, bankName string) ProgramDTO {
	return ProgramDTO{
		ID: p.ID, BankID: p.BankID, BankName: bankName, Name: p.Name,
		PeriodType: string(p.PeriodType), SelectionMode: string(p.SelectionMode),
		CurrencyKind: string(p.CurrencyKind), PointsLabel: p.PointsLabel,
		SelectionOpensDay: p.SelectionOpensDay, Notes: p.Notes,
	}
}

type TierDTO struct {
	ID                 int64   `json:"id"`
	ProgramID          int64   `json:"program_id"`
	Name               string  `json:"name"`
	IsPaidSubscription bool    `json:"is_paid_subscription"`
	CapValue           *string `json:"cap_value,omitempty"`
	CapScope           string  `json:"cap_scope"`
	CapPerCategory     *string `json:"cap_per_category,omitempty"`
	MaxCategories      *int32  `json:"max_categories,omitempty"`
	BasePercent        *string `json:"base_percent,omitempty"`
	Notes              *string `json:"notes,omitempty"`
}

func tierDTO(t db.ProgramTier) TierDTO {
	return TierDTO{
		ID: t.ID, ProgramID: t.ProgramID, Name: t.Name, IsPaidSubscription: t.IsPaidSubscription,
		CapValue: decToStr(t.CapValue), CapScope: string(t.CapScope),
		CapPerCategory: decToStr(t.CapPerCategory), MaxCategories: t.MaxCategories,
		BasePercent: decToStr(t.BasePercent), Notes: t.Notes,
	}
}

type programBody struct {
	BankID            int32   `json:"bank_id"`
	Name              string  `json:"name" minLength:"1"`
	PeriodType        string  `json:"period_type" enum:"calendar_month,quarter,week,rolling"`
	SelectionMode     string  `json:"selection_mode" enum:"atomic,incremental"`
	CurrencyKind      string  `json:"currency_kind" enum:"rub,points"`
	PointsLabel       *string `json:"points_label,omitempty"`
	SelectionOpensDay *int32  `json:"selection_opens_day,omitempty" minimum:"1" maximum:"31"`
	Notes             *string `json:"notes,omitempty"`
}

type tierBody struct {
	ProgramID          int64   `json:"program_id"`
	Name               string  `json:"name" minLength:"1"`
	IsPaidSubscription bool    `json:"is_paid_subscription,omitempty"`
	CapValue           *string `json:"cap_value,omitempty"`
	CapScope           string  `json:"cap_scope,omitempty" enum:"total,per_category,both" default:"total"`
	CapPerCategory     *string `json:"cap_per_category,omitempty"`
	MaxCategories      *int32  `json:"max_categories,omitempty" minimum:"1"`
	BasePercent        *string `json:"base_percent,omitempty"`
	Notes              *string `json:"notes,omitempty"`
}

type OfferPeriodDTO struct {
	ID                    int64  `json:"id"`
	BankClientID          int64  `json:"bank_client_id"`
	PeriodStart           string `json:"period_start"`
	PeriodEnd             string `json:"period_end"`
	MaxCategoriesOverride *int32 `json:"max_categories_override,omitempty"`
}

func offerPeriodDTO(p db.OfferPeriod) OfferPeriodDTO {
	return OfferPeriodDTO{
		ID: p.ID, BankClientID: p.BankClientID,
		PeriodStart:           p.PeriodStart.Format("2006-01-02"),
		PeriodEnd:             p.PeriodEnd.Format("2006-01-02"),
		MaxCategoriesOverride: p.MaxCategoriesOverride,
	}
}

type categoryOfferBody struct {
	RawTitle            string  `json:"raw_title" minLength:"1"`
	CanonicalCategoryID *int64  `json:"canonical_category_id,omitempty"`
	BankCategoryID      *int64  `json:"bank_category_id,omitempty"`
	Percent             *string `json:"percent,omitempty"`
	Kind                string  `json:"kind,omitempty" enum:"regular,super,special" default:"regular"`
	Notes               *string `json:"notes,omitempty"`
}

type CategoryOfferDTO struct {
	ID                  int64      `json:"id"`
	OfferPeriodID       int64      `json:"offer_period_id"`
	RawTitle            string     `json:"raw_title"`
	CanonicalCategoryID *int64     `json:"canonical_category_id,omitempty"`
	BankCategoryID      *int64     `json:"bank_category_id,omitempty"`
	Percent             *string    `json:"percent,omitempty"`
	Kind                string     `json:"kind"`
	Notes               *string    `json:"notes,omitempty"`
	SelectionID         *int64     `json:"selection_id,omitempty"`
	SelectedAt          *time.Time `json:"selected_at,omitempty"`
}

// OfferPeriodListItem is one row of the period list (needs a name so huma
// doesn't auto-name slice elements and collide).
type OfferPeriodListItem struct {
	OfferPeriodDTO
	BankName string `json:"bank_name"`
}

// HelperRowDTO is one menu row in the helper-context response.
type HelperRowDTO struct {
	CategoryOfferID int64           `json:"category_offer_id"`
	RawTitle        string          `json:"raw_title"`
	Percent         *string         `json:"percent,omitempty"`
	Kind            string          `json:"kind"`
	Selected        bool            `json:"selected"`
	Collisions      []CollisionDTO  `json:"collisions,omitempty"`
	Comparisons     []ComparisonDTO `json:"comparisons,omitempty"`
}

type CollisionDTO struct {
	BankName    string  `json:"bank_name"`
	ClientLabel string  `json:"client_label"`
	HolderLabel string  `json:"holder_label,omitempty"`
	Percent     *string `json:"percent,omitempty"`
	CapNote     string  `json:"cap_note,omitempty"`
	Message     string  `json:"message"`
}

type ComparisonDTO struct {
	BankName    string  `json:"bank_name"`
	ClientLabel string  `json:"client_label"`
	RawTitle    string  `json:"raw_title"`
	Percent     *string `json:"percent,omitempty"`
	Selected    bool    `json:"selected"`
}

type LookupEntryDTO struct {
	BankClientID   int64   `json:"bank_client_id"`
	BankName       string  `json:"bank_name"`
	ClientLabel    string  `json:"client_label"`
	HolderLabel    string  `json:"holder_label,omitempty"`
	Kind           string  `json:"kind"` // regular | super | special — lets the UI mark the ranked super (барабан)
	Percent        *string `json:"percent,omitempty"`
	CurrencyKind   string  `json:"currency_kind"`
	PointsLabel    string  `json:"points_label,omitempty"`
	CapValue       *string `json:"cap_value,omitempty"`
	CapPerCategory *string `json:"cap_per_category,omitempty"`
	CapScope       string  `json:"cap_scope,omitempty"`
	PeriodStart    string  `json:"period_start"`
	PeriodEnd      string  `json:"period_end"`
}

func lookupEntryDTO(e LookupEntry) LookupEntryDTO {
	return LookupEntryDTO{
		BankClientID: e.ClientID, Kind: string(e.Kind),
		BankName: e.BankName, ClientLabel: e.ClientLabel, HolderLabel: e.HolderLabel, Percent: decToStr(e.Percent),
		CurrencyKind: string(e.CurrencyKind), PointsLabel: e.PointsLabel,
		CapValue: decToStr(e.CapValue), CapPerCategory: decToStr(e.CapPerCategory),
		CapScope:    string(e.CapScope),
		PeriodStart: e.Period.Start.Format("2006-01-02"),
		PeriodEnd:   e.Period.End.Format("2006-01-02"),
	}
}

// AvailableEntryDTO is one S3b «Можно выбрать» row: an offered-but-unselected
// menu offer with a fact-based verdict (spec S3b, owner 2026-07-16).
type AvailableEntryDTO struct {
	LookupEntryDTO
	OfferID    int64  `json:"offer_id" doc:"category_offer id — «Отметить выбранной» posts the ordinary selection for it"`
	RawTitle   string `json:"raw_title"`
	Verdict    string `json:"verdict" enum:"free,paid,locked,slots_full,unknown"`
	Activation string `json:"activation" enum:"immediate,next_day,unknown" doc:"next_day (МКБ): a fresh pick won't cover a purchase made right now"`
}

type PartnerOfferDTO struct {
	ID            int64   `json:"id"`
	BankID        int32   `json:"bank_id"`
	BankName      string  `json:"bank_name,omitempty"`
	BankClientID  *int64  `json:"bank_client_id,omitempty"`
	MerchantTitle string  `json:"merchant_title"`
	Percent       *string `json:"percent,omitempty"`
	ValidFrom     *string `json:"valid_from,omitempty"`
	ValidTo       *string `json:"valid_to,omitempty"`
	CapValue      *string `json:"cap_value,omitempty"`
	Notes         *string `json:"notes,omitempty"`
}

func fmtDatePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}

type CanonicalCategoryDTO struct {
	ID      int64   `json:"id"`
	Slug    string  `json:"slug"`
	TitleRu string  `json:"title_ru"`
	Emoji   *string `json:"emoji,omitempty"`
}

func canonicalCategoryDTO(c db.CanonicalCategory) CanonicalCategoryDTO {
	return CanonicalCategoryDTO{ID: c.ID, Slug: c.Slug, TitleRu: c.TitleRu, Emoji: c.Emoji}
}

// BankCategoryDTO is one row of a bank's picker catalog. Emoji is resolved
// server-side: the row's own override, else the canonical's, else null (the
// SPA shows a generic fallback).
type BankCategoryDTO struct {
	ID                  int64   `json:"id"`
	BankID              int32   `json:"bank_id"`
	Title               string  `json:"title"`
	CanonicalCategoryID *int64  `json:"canonical_category_id,omitempty"`
	CanonicalSlug       *string `json:"canonical_slug,omitempty"`
	CanonicalTitleRu    *string `json:"canonical_title_ru,omitempty"`
	Kind                string  `json:"kind"` // regular | super | special — prefill hint for the entry form
	Emoji               *string `json:"emoji,omitempty"`
	IsCustom            bool    `json:"is_custom"`
}

// OverviewCategoryDTO is one «Категории» row: category + its best card.
type OverviewCategoryDTO struct {
	CategoryID  int64          `json:"category_id"`
	Slug        string         `json:"slug"`
	TitleRu     string         `json:"title_ru"`
	Best        LookupEntryDTO `json:"best"`
	OthersCount int            `json:"others_count"`
}

// OverviewChipDTO is a selected menu row rendered as a chip on a card.
type OverviewChipDTO struct {
	OfferID  int64   `json:"offer_id"`
	RawTitle string  `json:"raw_title"`
	Kind     string  `json:"kind"` // regular | super | special — «спец» vs «супер» chip label
	Percent  *string `json:"percent,omitempty"`
}

// OverviewBaseDTO is the «Остальное» row: the best base rate across clients.
type OverviewBaseDTO struct {
	Best        LookupEntryDTO `json:"best"`
	OthersCount int            `json:"others_count"`
}

// OverviewCardChipDTO is one plastic of the client — any of them pays with
// the client's shared selection.
type OverviewCardChipDTO struct {
	CardID        int32  `json:"card_id"`
	Last4Digits   int32  `json:"last_4_digits"`
	PaymentSystem string `json:"payment_system"`
}

// OverviewClientDTO is one «Карты» row — a bank client (person × bank) with
// its plastics; period fields are null when the client has no offer_period
// covering the date.
type OverviewClientDTO struct {
	BankClientID   int64                 `json:"bank_client_id"`
	BankID         int32                 `json:"bank_id"`
	BankName       string                `json:"bank_name"`
	HolderLabel    *string               `json:"holder_label,omitempty"`
	Cards          []OverviewCardChipDTO `json:"cards"`
	TierName       *string               `json:"tier_name,omitempty"`
	IsPaidTier     bool                  `json:"is_paid_tier,omitempty"`
	CapValue       *string               `json:"cap_value,omitempty"`
	CapPerCategory *string               `json:"cap_per_category,omitempty"`
	CapScope       string                `json:"cap_scope,omitempty"`
	CurrencyKind   string                `json:"currency_kind"`
	PointsLabel    string                `json:"points_label,omitempty"`
	SelectionMode  string                `json:"selection_mode,omitempty"`
	PeriodID       *int64                `json:"period_id,omitempty"`
	PeriodStart    *string               `json:"period_start,omitempty"`
	PeriodEnd      *string               `json:"period_end,omitempty"`
	SlotsUsed      int                   `json:"slots_used"`
	MaxCategories  *int32                `json:"max_categories,omitempty"`
	Selected       []OverviewChipDTO     `json:"selected"`
	Specials       []OverviewChipDTO     `json:"specials,omitempty"`
}

// RegisterHTTP mounts the module's API (spec «Interfaces & files»).
func RegisterHTTP(api huma.API, s *Service) {
	// --- programs / tiers (admin-ish reference data, shared across users) ---

	huma.Register(api, huma.Operation{
		OperationID: "cashback-program-create", Method: http.MethodPost,
		Path: "/api/v1/cashback/programs", Summary: "Create a cashback program", Tags: []string{"cashback"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Body programBody
	}) (*struct{ Body ProgramDTO }, error) {
		p, err := s.Q.CreateProgram(ctx, db.CreateProgramParams{
			BankID: in.Body.BankID, Name: in.Body.Name,
			PeriodType:    db.CashbackPeriodType(in.Body.PeriodType),
			SelectionMode: db.CashbackSelectionMode(in.Body.SelectionMode),
			CurrencyKind:  db.CashbackCurrencyKind(in.Body.CurrencyKind),
			PointsLabel:   in.Body.PointsLabel, SelectionOpensDay: in.Body.SelectionOpensDay, Notes: in.Body.Notes,
		})
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body ProgramDTO }{programDTO(p, "")}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-program-list", Method: http.MethodGet,
		Path: "/api/v1/cashback/programs", Summary: "List cashback programs", Tags: []string{"cashback"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []ProgramDTO }, error) {
		rows, err := s.Q.ListPrograms(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]ProgramDTO, len(rows))
		for i, r := range rows {
			out[i] = programDTO(db.CashbackProgram{
				ID: r.ID, BankID: r.BankID, Name: r.Name, PeriodType: r.PeriodType,
				SelectionMode: r.SelectionMode, CurrencyKind: r.CurrencyKind,
				PointsLabel: r.PointsLabel, SelectionOpensDay: r.SelectionOpensDay, Notes: r.Notes,
			}, r.BankName)
		}
		return &struct{ Body []ProgramDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-tier-create", Method: http.MethodPost,
		Path: "/api/v1/cashback/tiers", Summary: "Create a program tier (client level)", Tags: []string{"cashback"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Body tierBody
	}) (*struct{ Body TierDTO }, error) {
		capValue, err := strToDec(in.Body.CapValue, "cap_value")
		if err != nil {
			return nil, err
		}
		capPerCat, err := strToDec(in.Body.CapPerCategory, "cap_per_category")
		if err != nil {
			return nil, err
		}
		basePct, err := strToDec(in.Body.BasePercent, "base_percent")
		if err != nil {
			return nil, err
		}
		scope := in.Body.CapScope
		if scope == "" {
			scope = string(CapTotal)
		}
		t, err := s.Q.CreateTier(ctx, db.CreateTierParams{
			ProgramID: in.Body.ProgramID, Name: in.Body.Name,
			IsPaidSubscription: in.Body.IsPaidSubscription,
			CapValue:           capValue, CapScope: db.CashbackCapScope(scope),
			CapPerCategory: capPerCat, MaxCategories: in.Body.MaxCategories,
			BasePercent: basePct, Notes: in.Body.Notes,
		})
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body TierDTO }{tierDTO(t)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-tier-list", Method: http.MethodGet,
		Path: "/api/v1/cashback/programs/{id}/tiers", Summary: "List tiers of a program", Tags: []string{"cashback"},
	}, func(ctx context.Context, in *struct {
		ID int64 `path:"id"`
	}) (*struct{ Body []TierDTO }, error) {
		rows, err := s.Q.ListTiersForProgram(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		out := make([]TierDTO, len(rows))
		for i, r := range rows {
			out[i] = tierDTO(r)
		}
		return &struct{ Body []TierDTO }{out}, nil
	})

	// --- canonical categories ---

	huma.Register(api, huma.Operation{
		OperationID: "cashback-canonical-list", Method: http.MethodGet,
		Path: "/api/v1/cashback/canonical-categories", Summary: "List canonical categories", Tags: []string{"cashback"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []CanonicalCategoryDTO }, error) {
		rows, err := s.Q.ListCanonicalCategories(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]CanonicalCategoryDTO, len(rows))
		for i, r := range rows {
			out[i] = canonicalCategoryDTO(r)
		}
		return &struct{ Body []CanonicalCategoryDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-canonical-create", Method: http.MethodPost,
		Path: "/api/v1/cashback/canonical-categories", Summary: "Create a canonical category", Tags: []string{"cashback"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Body struct {
			Slug    string  `json:"slug" minLength:"1" pattern:"^[a-z0-9-]+$"`
			TitleRu string  `json:"title_ru" minLength:"1"`
			Emoji   *string `json:"emoji,omitempty"`
		}
	}) (*struct{ Body CanonicalCategoryDTO }, error) {
		c, err := s.Q.CreateCanonicalCategory(ctx, db.CreateCanonicalCategoryParams{Slug: in.Body.Slug, TitleRu: in.Body.TitleRu, Emoji: in.Body.Emoji})
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body CanonicalCategoryDTO }{canonicalCategoryDTO(c)}, nil
	})

	// --- bank picker catalogs ---

	huma.Register(api, huma.Operation{
		OperationID: "cashback-bank-category-list", Method: http.MethodGet,
		Path: "/api/v1/cashback/banks/{bank_id}/categories", Summary: "The bank's picker catalog (current menu rows)", Tags: []string{"cashback"},
	}, func(ctx context.Context, in *struct {
		BankID int32 `path:"bank_id"`
	}) (*struct{ Body []BankCategoryDTO }, error) {
		rows, err := s.ListBankCategories(ctx, in.BankID)
		if err != nil {
			return nil, err
		}
		out := make([]BankCategoryDTO, len(rows))
		for i, r := range rows {
			emoji := r.Emoji
			if emoji == nil {
				emoji = r.CanonicalEmoji
			}
			out[i] = BankCategoryDTO{
				ID: r.ID, BankID: r.BankID, Title: r.Title,
				CanonicalCategoryID: r.CanonicalCategoryID,
				CanonicalSlug:       r.CanonicalSlug, CanonicalTitleRu: r.CanonicalTitleRu,
				Kind: string(r.Kind), Emoji: emoji, IsCustom: r.IsCustom,
			}
		}
		return &struct{ Body []BankCategoryDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-bank-category-create", Method: http.MethodPost,
		Path: "/api/v1/cashback/bank-categories", Summary: "Add a custom category to a bank's picker catalog", Tags: []string{"cashback"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Body struct {
			BankID              int32   `json:"bank_id"`
			Title               string  `json:"title" minLength:"1"`
			CanonicalCategoryID *int64  `json:"canonical_category_id,omitempty"`
			Kind                string  `json:"kind,omitempty" enum:"regular,super,special" default:"regular"`
			Emoji               *string `json:"emoji,omitempty"`
		}
	}) (*struct{ Body BankCategoryDTO }, error) {
		kind := OfferKind(in.Body.Kind)
		if kind == "" {
			kind = OfferRegular
		}
		bc, err := s.CreateBankCategory(ctx, in.Body.BankID, in.Body.Title, in.Body.CanonicalCategoryID, kind, in.Body.Emoji)
		if err != nil {
			return nil, httpErr(err)
		}
		out := BankCategoryDTO{
			ID: bc.ID, BankID: bc.BankID, Title: bc.Title,
			CanonicalCategoryID: bc.CanonicalCategoryID,
			Kind:                string(bc.Kind), Emoji: bc.Emoji, IsCustom: bc.IsCustom,
		}
		// Resolve the canonical's title/slug/emoji like the list does, so
		// the SPA can use the fresh row without refetching.
		if bc.CanonicalCategoryID != nil {
			if cats, err := s.Q.ListCanonicalCategories(ctx); err == nil {
				for _, c := range cats {
					if c.ID == *bc.CanonicalCategoryID {
						out.CanonicalSlug, out.CanonicalTitleRu = &c.Slug, &c.TitleRu
						if out.Emoji == nil {
							out.Emoji = c.Emoji
						}
						break
					}
				}
			}
		}
		return &struct{ Body BankCategoryDTO }{out}, nil
	})

	// --- offer periods ---

	huma.Register(api, huma.Operation{
		OperationID: "cashback-offer-period-create", Method: http.MethodPost,
		Path: "/api/v1/cashback/offer-periods", Summary: "Open a selection period for a bank client", Tags: []string{"cashback"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Body struct {
			BankClientID  int64       `json:"bank_client_id"`
			PeriodStart   string      `json:"period_start" format:"date"`
			PeriodEnd     string      `json:"period_end" format:"date"`
			AttachmentIDs []uuid.UUID `json:"attachment_ids,omitempty"`
		}
	}) (*struct{ Body OfferPeriodDTO }, error) {
		start, err := parseDate(in.Body.PeriodStart, "period_start")
		if err != nil {
			return nil, err
		}
		end, err := parseDate(in.Body.PeriodEnd, "period_end")
		if err != nil {
			return nil, err
		}
		p, err := s.CreateOfferPeriod(ctx, auth.UserID(ctx), in.Body.BankClientID, start, end, in.Body.AttachmentIDs)
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body OfferPeriodDTO }{offerPeriodDTO(p)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-offer-period-list", Method: http.MethodGet,
		Path: "/api/v1/cashback/offer-periods", Summary: "List the user's selection periods", Tags: []string{"cashback"},
	}, func(ctx context.Context, in *struct {
		BankClientID int64 `query:"bank_client_id"`
	}) (*struct{ Body []OfferPeriodListItem }, error) {
		rows, err := s.Q.ListOfferPeriodsForUser(ctx, auth.UserID(ctx))
		if err != nil {
			return nil, err
		}
		out := &struct{ Body []OfferPeriodListItem }{}
		for _, r := range rows {
			if in.BankClientID != 0 && r.BankClientID != in.BankClientID {
				continue
			}
			out.Body = append(out.Body, OfferPeriodListItem{
				OfferPeriodDTO: offerPeriodDTO(db.OfferPeriod{ID: r.ID, BankClientID: r.BankClientID, PeriodStart: r.PeriodStart, PeriodEnd: r.PeriodEnd, MaxCategoriesOverride: r.MaxCategoriesOverride}),
				BankName:       r.BankName,
			})
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-offer-period-get", Method: http.MethodGet,
		Path: "/api/v1/cashback/offer-periods/{id}", Summary: "Selection period with its menu", Tags: []string{"cashback"},
	}, func(ctx context.Context, in *struct {
		ID int64 `path:"id"`
	}) (*struct {
		Body struct {
			OfferPeriodDTO
			BankID      int32              `json:"bank_id"`
			BankName    string             `json:"bank_name"`
			Offers      []CategoryOfferDTO `json:"offers"`
			Attachments []uuid.UUID        `json:"attachment_ids"`
		}
	}, error) {
		p, err := s.Q.GetOfferPeriodForUser(ctx, db.GetOfferPeriodForUserParams{ID: in.ID, UserID: auth.UserID(ctx)})
		if err != nil {
			return nil, httpErr(notFound(err))
		}
		offers, err := s.Q.ListOffersForPeriod(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		atts, err := s.Q.ListOfferPeriodAttachments(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		out := &struct {
			Body struct {
				OfferPeriodDTO
				BankID      int32              `json:"bank_id"`
				BankName    string             `json:"bank_name"`
				Offers      []CategoryOfferDTO `json:"offers"`
				Attachments []uuid.UUID        `json:"attachment_ids"`
			}
		}{}
		out.Body.OfferPeriodDTO = offerPeriodDTO(db.OfferPeriod{ID: p.ID, BankClientID: p.BankClientID, PeriodStart: p.PeriodStart, PeriodEnd: p.PeriodEnd, MaxCategoriesOverride: p.MaxCategoriesOverride})
		out.Body.BankID = int32(p.BankID)
		out.Body.BankName = p.BankName
		out.Body.Offers = make([]CategoryOfferDTO, len(offers))
		for i, o := range offers {
			out.Body.Offers[i] = CategoryOfferDTO{
				ID: o.ID, OfferPeriodID: o.OfferPeriodID, RawTitle: o.RawTitle,
				CanonicalCategoryID: o.CanonicalCategoryID, BankCategoryID: o.BankCategoryID,
				Percent: decToStr(o.Percent),
				Kind:    string(o.Kind), Notes: o.Notes, SelectionID: o.SelectionID,
				SelectedAt: o.SelectedAt,
			}
		}
		out.Body.Attachments = make([]uuid.UUID, len(atts))
		for i, a := range atts {
			out.Body.Attachments[i] = a.ID
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-offer-period-attach", Method: http.MethodPost,
		Path: "/api/v1/cashback/offer-periods/{id}/attachments", Summary: "Attach a screenshot to the period", Tags: []string{"cashback"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID   int64 `path:"id"`
		Body struct {
			AttachmentID uuid.UUID `json:"attachment_id"`
		}
	}) (*struct{}, error) {
		if err := s.AttachScreenshot(ctx, auth.UserID(ctx), in.ID, in.Body.AttachmentID); err != nil {
			return nil, httpErr(err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-offer-period-detach", Method: http.MethodDelete,
		Path: "/api/v1/cashback/offer-periods/{id}/attachments/{attachment_id}", Summary: "Detach a screenshot (orphaned file is removed)", Tags: []string{"cashback"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID           int64     `path:"id"`
		AttachmentID uuid.UUID `path:"attachment_id"`
	}) (*struct{}, error) {
		if err := s.DetachScreenshot(ctx, auth.UserID(ctx), in.ID, in.AttachmentID); err != nil {
			return nil, httpErr(err)
		}
		return &struct{}{}, nil
	})

	// --- alias suggestion + category offers ---

	huma.Register(api, huma.Operation{
		OperationID: "cashback-alias-suggestion", Method: http.MethodGet,
		Path: "/api/v1/cashback/alias-suggestion", Summary: "Suggest a canonical category for a raw menu title (S1)", Tags: []string{"cashback"},
	}, func(ctx context.Context, in *struct {
		OfferPeriodID int64  `query:"offer_period_id" required:"true"`
		RawTitle      string `query:"raw_title" required:"true"`
	}) (*struct {
		Body struct {
			Suggestion *CanonicalCategoryDTO `json:"suggestion"`
		}
	}, error) {
		c, err := s.SuggestAlias(ctx, auth.UserID(ctx), in.OfferPeriodID, in.RawTitle)
		if err != nil {
			return nil, httpErr(err)
		}
		out := &struct {
			Body struct {
				Suggestion *CanonicalCategoryDTO `json:"suggestion"`
			}
		}{}
		if c != nil {
			dto := canonicalCategoryDTO(*c)
			out.Body.Suggestion = &dto
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-category-offer-create", Method: http.MethodPost,
		Path: "/api/v1/cashback/category-offers", Summary: "Record one row of a bank's offer menu", Tags: []string{"cashback"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Body struct {
			OfferPeriodID       int64   `json:"offer_period_id"`
			RawTitle            string  `json:"raw_title" minLength:"1"`
			CanonicalCategoryID *int64  `json:"canonical_category_id,omitempty"`
			BankCategoryID      *int64  `json:"bank_category_id,omitempty"`
			Percent             *string `json:"percent,omitempty"`
			Kind                string  `json:"kind,omitempty" enum:"regular,super,special" default:"regular"`
			Notes               *string `json:"notes,omitempty"`
		}
	}) (*struct{ Body CategoryOfferDTO }, error) {
		pctVal, err := strToDec(in.Body.Percent, "percent")
		if err != nil {
			return nil, err
		}
		kind := OfferKind(in.Body.Kind)
		if kind == "" {
			kind = OfferRegular
		}
		o, err := s.CreateCategoryOffer(ctx, auth.UserID(ctx), in.Body.OfferPeriodID, in.Body.RawTitle, in.Body.CanonicalCategoryID, pctVal, kind, in.Body.Notes, in.Body.BankCategoryID)
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body CategoryOfferDTO }{CategoryOfferDTO{
			ID: o.ID, OfferPeriodID: o.OfferPeriodID, RawTitle: o.RawTitle,
			CanonicalCategoryID: o.CanonicalCategoryID, BankCategoryID: o.BankCategoryID,
			Percent: decToStr(o.Percent),
			Kind:    string(o.Kind), Notes: o.Notes,
		}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-category-offer-update", Method: http.MethodPut,
		Path: "/api/v1/cashback/category-offers/{id}", Summary: "Edit a menu row (full replace of mutable fields)", Tags: []string{"cashback"},
	}, func(ctx context.Context, in *struct {
		ID   int64 `path:"id"`
		Body categoryOfferBody
	}) (*struct{ Body CategoryOfferDTO }, error) {
		pctVal, err := strToDec(in.Body.Percent, "percent")
		if err != nil {
			return nil, err
		}
		kind := OfferKind(in.Body.Kind)
		if kind == "" {
			kind = OfferRegular
		}
		o, err := s.UpdateCategoryOffer(ctx, auth.UserID(ctx), in.ID, in.Body.RawTitle, in.Body.CanonicalCategoryID, pctVal, kind, in.Body.Notes, in.Body.BankCategoryID)
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body CategoryOfferDTO }{CategoryOfferDTO{
			ID: o.ID, OfferPeriodID: o.OfferPeriodID, RawTitle: o.RawTitle,
			CanonicalCategoryID: o.CanonicalCategoryID, BankCategoryID: o.BankCategoryID,
			Percent: decToStr(o.Percent),
			Kind:    string(o.Kind), Notes: o.Notes,
		}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-category-offer-delete", Method: http.MethodDelete,
		Path: "/api/v1/cashback/category-offers/{id}", Summary: "Delete a menu row (with its selection)", Tags: []string{"cashback"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID int64 `path:"id"`
	}) (*struct{}, error) {
		if err := s.DeleteCategoryOffer(ctx, auth.UserID(ctx), in.ID); err != nil {
			return nil, httpErr(err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-offer-period-slots", Method: http.MethodPut,
		Path: "/api/v1/cashback/offer-periods/{id}/max-categories", Summary: "Set the period's slot count (null resets to the tier default)", Tags: []string{"cashback"},
	}, func(ctx context.Context, in *struct {
		ID   int64 `path:"id"`
		Body struct {
			Value *int32 `json:"value" minimum:"1"`
		}
	}) (*struct{ Body OfferPeriodDTO }, error) {
		p, err := s.SetPeriodMaxOverride(ctx, auth.UserID(ctx), in.ID, in.Body.Value)
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body OfferPeriodDTO }{offerPeriodDTO(p)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-offer-period-delete", Method: http.MethodDelete,
		Path: "/api/v1/cashback/offer-periods/{id}", Summary: "Delete a period with its menu and selections", Tags: []string{"cashback"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID int64 `path:"id"`
	}) (*struct{}, error) {
		if err := s.DeleteOfferPeriod(ctx, auth.UserID(ctx), in.ID); err != nil {
			return nil, httpErr(err)
		}
		return &struct{}{}, nil
	})

	// --- selections ---

	huma.Register(api, huma.Operation{
		OperationID: "cashback-selection-create", Method: http.MethodPost,
		Path: "/api/v1/cashback/selections", Summary: "Select an offer (dated event; invariants 1–2 enforced)", Tags: []string{"cashback"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Body struct {
			CategoryOfferID  int64      `json:"category_offer_id"`
			SelectedAt       *time.Time `json:"selected_at,omitempty"`
			BackfillOverride bool       `json:"backfill_override,omitempty"`
		}
	}) (*struct {
		Body struct {
			ID              int64     `json:"id"`
			CategoryOfferID int64     `json:"category_offer_id"`
			SelectedAt      time.Time `json:"selected_at"`
		}
	}, error) {
		at := time.Now()
		if in.Body.SelectedAt != nil {
			at = *in.Body.SelectedAt
		}
		sel, err := s.CreateSelection(ctx, auth.UserID(ctx), in.Body.CategoryOfferID, at, in.Body.BackfillOverride)
		if err != nil {
			return nil, httpErr(err)
		}
		out := &struct {
			Body struct {
				ID              int64     `json:"id"`
				CategoryOfferID int64     `json:"category_offer_id"`
				SelectedAt      time.Time `json:"selected_at"`
			}
		}{}
		out.Body.ID = sel.ID
		out.Body.CategoryOfferID = sel.CategoryOfferID
		out.Body.SelectedAt = sel.SelectedAt
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-selection-delete", Method: http.MethodDelete,
		Path: "/api/v1/cashback/selections/{id}", Summary: "Undo a selection", Tags: []string{"cashback"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID int64 `path:"id"`
	}) (*struct{}, error) {
		if err := s.DeleteSelection(ctx, auth.UserID(ctx), in.ID); err != nil {
			return nil, httpErr(err)
		}
		return &struct{}{}, nil
	})

	// --- helper context (S1 entry screen) ---

	huma.Register(api, huma.Operation{
		OperationID: "cashback-helper-context", Method: http.MethodGet,
		Path: "/api/v1/cashback/helper-context", Summary: "Collisions + comparisons + slot tracking for the entry screen", Tags: []string{"cashback"},
	}, func(ctx context.Context, in *struct {
		OfferPeriodID int64 `query:"offer_period_id" required:"true"`
	}) (*struct {
		Body struct {
			OfferPeriodID         int64          `json:"offer_period_id"`
			ClientLabel           string         `json:"client_label"`
			SlotsUsed             int            `json:"slots_used"`
			MaxCategories         *int32         `json:"max_categories,omitempty" doc:"effective limit: period override, else tier default"`
			MaxCategoriesOverride *int32         `json:"max_categories_override,omitempty"`
			Rows                  []HelperRowDTO `json:"rows"`
		}
	}, error) {
		res, err := s.HelperContext(ctx, auth.UserID(ctx), in.OfferPeriodID)
		if err != nil {
			return nil, httpErr(err)
		}
		out := &struct {
			Body struct {
				OfferPeriodID         int64          `json:"offer_period_id"`
				ClientLabel           string         `json:"client_label"`
				SlotsUsed             int            `json:"slots_used"`
				MaxCategories         *int32         `json:"max_categories,omitempty" doc:"effective limit: period override, else tier default"`
				MaxCategoriesOverride *int32         `json:"max_categories_override,omitempty"`
				Rows                  []HelperRowDTO `json:"rows"`
			}
		}{}
		out.Body.OfferPeriodID = res.Period.ID
		out.Body.ClientLabel = clientLabel(res.Period.BankName, res.Period.HolderLabel)
		out.Body.SlotsUsed = res.SlotsUsed
		out.Body.MaxCategories = res.MaxCategories
		out.Body.MaxCategoriesOverride = res.Override
		for _, r := range res.Rows {
			dto := HelperRowDTO{
				CategoryOfferID: r.Offer.ID, RawTitle: r.Offer.RawTitle,
				Percent: decToStr(r.Offer.Percent), Kind: string(r.Offer.Kind),
				Selected: r.Offer.SelectionID != nil,
			}
			for _, c := range r.Collisions {
				onWhom := c.Other.BankName
				if c.Other.HolderLabel != "" {
					onWhom += " (" + c.Other.HolderLabel + ")"
				}
				msg := fmt.Sprintf("«%s» уже выбраны на %s", r.Offer.RawTitle, onWhom)
				var details []string
				if c.Other.Percent != nil {
					details = append(details, c.Other.Percent.String()+"%")
				}
				if c.Other.CapNote != "" {
					details = append(details, c.Other.CapNote)
				}
				if len(details) > 0 {
					msg += " (" + details[0]
					for _, d := range details[1:] {
						msg += ", " + d
					}
					msg += ")"
				}
				dto.Collisions = append(dto.Collisions, CollisionDTO{
					BankName: c.Other.BankName, ClientLabel: c.Other.ClientLabel, HolderLabel: c.Other.HolderLabel,
					Percent: decToStr(c.Other.Percent), CapNote: c.Other.CapNote, Message: msg,
				})
			}
			for _, cmp := range r.Comparisons {
				dto.Comparisons = append(dto.Comparisons, ComparisonDTO{
					BankName: cmp.BankName, ClientLabel: cmp.ClientLabel,
					RawTitle: cmp.RawTitle, Percent: decToStr(cmp.Percent),
				})
			}
			out.Body.Rows = append(out.Body.Rows, dto)
		}
		return out, nil
	})

	// --- overview (design screens 01/02: Категории / Карты cuts) ---

	huma.Register(api, huma.Operation{
		OperationID: "cashback-overview", Method: http.MethodGet,
		Path: "/api/v1/cashback/overview", Summary: "Обзор кешбека: лучшие карты по категориям и срез по картам", Tags: []string{"cashback"},
	}, func(ctx context.Context, in *struct {
		Date string `query:"date" doc:"YYYY-MM-DD; defaults to today"`
	}) (*struct {
		Body struct {
			Date              string                `json:"date"`
			Categories        []OverviewCategoryDTO `json:"categories"`
			Base              *OverviewBaseDTO      `json:"base,omitempty"`
			Clients           []OverviewClientDTO   `json:"clients"`
			SelectionOpensDay *int32                `json:"selection_opens_day,omitempty"`
		}
	}, error) {
		onDate := time.Now()
		if in.Date != "" {
			var err error
			if onDate, err = parseDate(in.Date, "date"); err != nil {
				return nil, err
			}
		}
		res, err := s.Overview(ctx, auth.UserID(ctx), onDate)
		if err != nil {
			return nil, httpErr(err)
		}
		out := &struct {
			Body struct {
				Date              string                `json:"date"`
				Categories        []OverviewCategoryDTO `json:"categories"`
				Base              *OverviewBaseDTO      `json:"base,omitempty"`
				Clients           []OverviewClientDTO   `json:"clients"`
				SelectionOpensDay *int32                `json:"selection_opens_day,omitempty"`
			}
		}{}
		out.Body.Date = onDate.Format("2006-01-02")
		out.Body.SelectionOpensDay = res.SelectionOpensDay
		if res.Base != nil {
			out.Body.Base = &OverviewBaseDTO{Best: lookupEntryDTO(res.Base.Best), OthersCount: res.Base.OthersCount}
		}
		out.Body.Categories = make([]OverviewCategoryDTO, len(res.Categories))
		for i, g := range res.Categories {
			out.Body.Categories[i] = OverviewCategoryDTO{
				CategoryID: g.CategoryID, Slug: g.Slug, TitleRu: g.TitleRu,
				Best: lookupEntryDTO(g.Best), OthersCount: g.OthersCount,
			}
		}
		out.Body.Clients = make([]OverviewClientDTO, len(res.Clients))
		for i, c := range res.Clients {
			dto := OverviewClientDTO{
				BankClientID: c.ClientID, BankID: c.BankID, BankName: c.BankName,
				HolderLabel: c.HolderLabel, TierName: c.TierName, IsPaidTier: c.IsPaidTier,
				CapValue: decToStr(c.CapValue), CapPerCategory: decToStr(c.CapPerCat),
				CapScope: string(c.CapScope), CurrencyKind: string(c.CurrencyKind),
				PointsLabel: c.PointsLabel, SelectionMode: c.SelectionMode,
				PeriodID: c.PeriodID, SlotsUsed: c.SlotsUsed, MaxCategories: c.MaxCategories,
			}
			dto.Cards = make([]OverviewCardChipDTO, len(c.Cards))
			for j, cc := range c.Cards {
				dto.Cards[j] = OverviewCardChipDTO{CardID: cc.CardID, Last4Digits: cc.Last4Digits, PaymentSystem: cc.PaymentSystem}
			}
			if c.PeriodStart != nil {
				s := c.PeriodStart.Format("2006-01-02")
				dto.PeriodStart = &s
			}
			if c.PeriodEnd != nil {
				e := c.PeriodEnd.Format("2006-01-02")
				dto.PeriodEnd = &e
			}
			dto.Selected = make([]OverviewChipDTO, len(c.Selected))
			for j, r := range c.Selected {
				dto.Selected[j] = OverviewChipDTO{OfferID: r.OfferID, RawTitle: r.RawTitle, Kind: string(r.Kind), Percent: decToStr(r.Percent)}
			}
			for _, r := range c.Specials {
				dto.Specials = append(dto.Specials, OverviewChipDTO{OfferID: r.OfferID, RawTitle: r.RawTitle, Kind: string(r.Kind), Percent: decToStr(r.Percent)})
			}
			out.Body.Clients[i] = dto
		}
		return out, nil
	})

	// --- lookup (S3) ---

	huma.Register(api, huma.Operation{
		OperationID: "cashback-lookup", Method: http.MethodGet,
		Path: "/api/v1/cashback/lookup", Summary: "«Какой картой платить?» for a canonical category", Tags: []string{"cashback"},
	}, func(ctx context.Context, in *struct {
		Category string `query:"category" required:"true" doc:"canonical category slug"`
		Date     string `query:"date" doc:"YYYY-MM-DD; defaults to today"`
	}) (*struct {
		Body struct {
			Category  CanonicalCategoryDTO `json:"category"`
			Date      string               `json:"date"`
			Ranked    []LookupEntryDTO     `json:"ranked"`
			Special   []LookupEntryDTO     `json:"special,omitempty"`
			Fallback  []LookupEntryDTO     `json:"fallback,omitempty" doc:"selected «За все покупки» — pays when nothing ranks"`
			Available []AvailableEntryDTO  `json:"available,omitempty" doc:"S3b: offered-but-unselected menu rows, actionable verdicts first"`
			Partner   []PartnerOfferDTO    `json:"partner,omitempty"`
			Message   string               `json:"message,omitempty"`
		}
	}, error) {
		onDate := time.Now()
		if in.Date != "" {
			var err error
			if onDate, err = parseDate(in.Date, "date"); err != nil {
				return nil, err
			}
		}
		res, err := s.Lookup(ctx, auth.UserID(ctx), in.Category, onDate)
		if err != nil {
			return nil, httpErr(err)
		}
		out := &struct {
			Body struct {
				Category  CanonicalCategoryDTO `json:"category"`
				Date      string               `json:"date"`
				Ranked    []LookupEntryDTO     `json:"ranked"`
				Special   []LookupEntryDTO     `json:"special,omitempty"`
				Fallback  []LookupEntryDTO     `json:"fallback,omitempty" doc:"selected «За все покупки» — pays when nothing ranks"`
				Available []AvailableEntryDTO  `json:"available,omitempty" doc:"S3b: offered-but-unselected menu rows, actionable verdicts first"`
				Partner   []PartnerOfferDTO    `json:"partner,omitempty"`
				Message   string               `json:"message,omitempty"`
			}
		}{}
		out.Body.Category = CanonicalCategoryDTO{ID: res.Category.ID, Slug: res.Category.Slug, TitleRu: res.Category.TitleRu, Emoji: res.Category.Emoji}
		out.Body.Date = onDate.Format("2006-01-02")
		out.Body.Ranked = make([]LookupEntryDTO, len(res.Ranked))
		for i, e := range res.Ranked {
			out.Body.Ranked[i] = lookupEntryDTO(e)
		}
		for _, e := range res.Special {
			out.Body.Special = append(out.Body.Special, lookupEntryDTO(e))
		}
		for _, e := range res.Fallback {
			out.Body.Fallback = append(out.Body.Fallback, lookupEntryDTO(e))
		}
		for _, a := range res.Available {
			out.Body.Available = append(out.Body.Available, AvailableEntryDTO{
				LookupEntryDTO: lookupEntryDTO(a.Entry),
				OfferID:        a.OfferID, RawTitle: a.RawTitle,
				Verdict: string(a.Verdict), Activation: string(a.Activation),
			})
		}
		for _, p := range res.Partner {
			out.Body.Partner = append(out.Body.Partner, PartnerOfferDTO{
				ID: p.ID, BankID: p.BankID, BankName: p.BankName, BankClientID: p.BankClientID,
				MerchantTitle: p.MerchantTitle, Percent: decToStr(p.Percent),
				ValidFrom: fmtDatePtr(p.ValidFrom), ValidTo: fmtDatePtr(p.ValidTo),
				CapValue: decToStr(p.CapValue), Notes: p.Notes,
			})
		}
		// The dead-end message only when there is truly nothing — ranked,
		// special AND available all empty (spec S3b).
		if len(out.Body.Ranked) == 0 && len(out.Body.Special) == 0 && len(out.Body.Available) == 0 {
			out.Body.Message = "нет активных выборов"
		}
		return out, nil
	})

	// --- partner offers (record-only, S4) ---

	huma.Register(api, huma.Operation{
		OperationID: "cashback-partner-offer-create", Method: http.MethodPost,
		Path: "/api/v1/cashback/partner-offers", Summary: "Record a partner offer", Tags: []string{"cashback"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Body struct {
			BankID        int32       `json:"bank_id"`
			BankClientID  *int64      `json:"bank_client_id,omitempty"`
			MerchantTitle string      `json:"merchant_title" minLength:"1"`
			Percent       *string     `json:"percent,omitempty"`
			ValidFrom     *string     `json:"valid_from,omitempty" format:"date"`
			ValidTo       *string     `json:"valid_to,omitempty" format:"date"`
			CapValue      *string     `json:"cap_value,omitempty"`
			Notes         *string     `json:"notes,omitempty"`
			AttachmentIDs []uuid.UUID `json:"attachment_ids,omitempty"`
		}
	}) (*struct{ Body PartnerOfferDTO }, error) {
		pctVal, err := strToDec(in.Body.Percent, "percent")
		if err != nil {
			return nil, err
		}
		capVal, err := strToDec(in.Body.CapValue, "cap_value")
		if err != nil {
			return nil, err
		}
		var from, to *time.Time
		if in.Body.ValidFrom != nil {
			t, err := parseDate(*in.Body.ValidFrom, "valid_from")
			if err != nil {
				return nil, err
			}
			from = &t
		}
		if in.Body.ValidTo != nil {
			t, err := parseDate(*in.Body.ValidTo, "valid_to")
			if err != nil {
				return nil, err
			}
			to = &t
		}
		p, err := s.Q.CreatePartnerOffer(ctx, db.CreatePartnerOfferParams{
			UserID: auth.UserID(ctx), BankID: in.Body.BankID, BankClientID: in.Body.BankClientID,
			MerchantTitle: in.Body.MerchantTitle, Percent: pctVal,
			ValidFrom: from, ValidTo: to, CapValue: capVal, Notes: in.Body.Notes,
		})
		if err != nil {
			return nil, httpErr(err)
		}
		for _, aid := range in.Body.AttachmentIDs {
			userID := auth.UserID(ctx)
			if _, err := s.Q.GetAttachmentForUser(ctx, db.GetAttachmentForUserParams{ID: aid, UserID: &userID}); err != nil {
				return nil, httpErr(notFound(err))
			}
			if err := s.Q.AttachToPartnerOffer(ctx, db.AttachToPartnerOfferParams{PartnerOfferID: p.ID, AttachmentID: aid}); err != nil {
				return nil, err
			}
		}
		return &struct{ Body PartnerOfferDTO }{PartnerOfferDTO{
			ID: p.ID, BankID: p.BankID, BankClientID: p.BankClientID, MerchantTitle: p.MerchantTitle,
			Percent: decToStr(p.Percent), ValidFrom: fmtDatePtr(p.ValidFrom), ValidTo: fmtDatePtr(p.ValidTo),
			CapValue: decToStr(p.CapValue), Notes: p.Notes,
		}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-partner-offer-list", Method: http.MethodGet,
		Path: "/api/v1/cashback/partner-offers", Summary: "List partner offers", Tags: []string{"cashback"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []PartnerOfferDTO }, error) {
		rows, err := s.Q.ListPartnerOffersForUser(ctx, auth.UserID(ctx))
		if err != nil {
			return nil, err
		}
		out := make([]PartnerOfferDTO, len(rows))
		for i, p := range rows {
			out[i] = PartnerOfferDTO{
				ID: p.ID, BankID: p.BankID, BankName: p.BankName, BankClientID: p.BankClientID,
				MerchantTitle: p.MerchantTitle, Percent: decToStr(p.Percent),
				ValidFrom: fmtDatePtr(p.ValidFrom), ValidTo: fmtDatePtr(p.ValidTo),
				CapValue: decToStr(p.CapValue), Notes: p.Notes,
			}
		}
		return &struct{ Body []PartnerOfferDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-partner-offer-delete", Method: http.MethodDelete,
		Path: "/api/v1/cashback/partner-offers/{id}", Summary: "Delete a partner offer", Tags: []string{"cashback"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID int64 `path:"id"`
	}) (*struct{}, error) {
		n, err := s.Q.DeletePartnerOfferForUser(ctx, db.DeletePartnerOfferForUserParams{ID: in.ID, UserID: auth.UserID(ctx)})
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, huma.Error404NotFound("not found")
		}
		return &struct{}{}, nil
	})
}
