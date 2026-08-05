package cashback

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/sqkrv/sharespences/internal/auth"
	"github.com/sqkrv/sharespences/internal/db"
	"github.com/sqkrv/sharespences/internal/vision"
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

// Bounds on a money/percent field, both checked before the value can reach
// the driver. The columns are numeric(19,4), so nothing legitimate comes close
// to either limit — but shopspring's parser accepts a 500 000-digit integer
// and an exponent up to MaxInt32, and pgx then materialises whichever it gets
// as a big.Int while encoding the statement. Measured on this code: `1e-200000`
// cost 1.6 s of server time for a 40-byte body and was accepted (201); larger
// exponents do not come back at all, and http.Server sets no WriteTimeout, so
// nothing cuts the request off.
const (
	maxDecimalLen = 32  // digits a human types, with room to spare
	maxDecimalExp = 100 // numeric(19,4) needs -4..0
)

func strToDec(s *string, field string) (*decimal.Decimal, error) {
	if s == nil {
		return nil, nil
	}
	notANumber := huma.Error422UnprocessableEntity(fmt.Sprintf("%s: «%s» — не число", field, *s))
	if len(*s) > maxDecimalLen {
		return nil, notANumber
	}
	// RU keyboards type «1,5» — the comma is a decimal separator here.
	d, err := decimal.NewFromString(strings.ReplaceAll(*s, ",", "."))
	if err != nil {
		return nil, notANumber
	}
	if e := d.Exponent(); e < -maxDecimalExp || e > maxDecimalExp {
		return nil, notANumber
	}
	return &d, nil
}

func parseDate(s string, field string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, huma.Error422UnprocessableEntity(fmt.Sprintf("%s: ожидается дата вида ГГГГ-ММ-ДД, получено «%s»", field, s))
	}
	return t, nil
}

func httpErr(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return huma.Error404NotFound("не найдено")
	case errors.Is(err, ErrSlotsExhausted),
		errors.Is(err, ErrAlreadySelected),
		errors.Is(err, ErrPeriodOverlap):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, ErrBankCategoryExists):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, ErrOutsidePeriod), errors.Is(err, ErrInvalidPeriod),
		errors.Is(err, ErrBankCategoryWrongBank), errors.Is(err, ErrRecognitionImages):
		return huma.Error422UnprocessableEntity(err.Error())
	case errors.Is(err, ErrRecognitionBusy):
		return huma.Error409Conflict(err.Error())
	// Vision failures must not surface as a bare 500 with no message:
	// backend absent/unreachable is honest degradation (manual entry
	// still works), a failed read is a per-job 422.
	case errors.Is(err, vision.ErrUnavailable):
		return huma.Error503ServiceUnavailable("распознавание сейчас недоступно — заполни период вручную")
	case errors.Is(err, vision.ErrFailed):
		return huma.Error422UnprocessableEntity("не удалось разобрать скриншот — попробуй другой снимок или заполни вручную")
	case errors.Is(err, vision.ErrBadImage):
		return huma.Error422UnprocessableEntity("файл не похож на изображение меню категорий")
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
	// The two policy axes selection_mode does NOT capture (migration 00008):
	// Альфа is atomic yet allows mid-period adds, ВТБ/Озон are atomic and
	// lock after the first confirm.
	MidPeriodAdd string  `json:"mid_period_add" enum:"allowed,locked_after_first,paid,unknown"`
	Activation   string  `json:"activation" enum:"immediate,next_day,unknown"`
	Notes        *string `json:"notes,omitempty"`
}

func programDTO(p db.CashbackProgram, bankName string) ProgramDTO {
	return ProgramDTO{
		ID: p.ID, BankID: p.BankID, BankName: bankName, Name: p.Name,
		PeriodType: string(p.PeriodType), SelectionMode: string(p.SelectionMode),
		CurrencyKind: string(p.CurrencyKind), PointsLabel: p.PointsLabel,
		SelectionOpensDay: p.SelectionOpensDay,
		MidPeriodAdd:      string(p.MidPeriodAdd), Activation: string(p.Activation),
		Notes: p.Notes,
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
	Notes              *string `json:"notes,omitempty"`
}

func tierDTO(t db.ProgramTier) TierDTO {
	return TierDTO{
		ID: t.ID, ProgramID: t.ProgramID, Name: t.Name, IsPaidSubscription: t.IsPaidSubscription,
		CapValue: decToStr(t.CapValue), CapScope: string(t.CapScope),
		CapPerCategory: decToStr(t.CapPerCategory), MaxCategories: t.MaxCategories, Notes: t.Notes,
	}
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
	CapValue            *string `json:"cap_value,omitempty" doc:"per-offer КБ cap for the period (ВТБ «Кешбэк до N ₽»); static display, no tracking"`
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
	CapValue            *string    `json:"cap_value,omitempty"`
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
	RawTitle       string  `json:"raw_title" doc:"the bank's own menu title — names the mechanic on marked super/special rows («Пятница»)"`
	Kind           string  `json:"kind"` // regular | super | special — all rank; the UI marks барабан/спец (amendment 2026-07-27)
	Percent        *string `json:"percent,omitempty"`
	CurrencyKind   string  `json:"currency_kind"`
	PointsLabel    string  `json:"points_label,omitempty"`
	CapValue       *string `json:"cap_value,omitempty"`
	CapPerCategory *string `json:"cap_per_category,omitempty"`
	CapScope       string  `json:"cap_scope,omitempty"`
	OfferCapValue  *string `json:"offer_cap_value,omitempty" doc:"per-offer cap (ВТБ «Кешбэк до N ₽»); display it over the tier cap"`
	PeriodStart    string  `json:"period_start"`
	PeriodEnd      string  `json:"period_end"`
	StackedRegular *string `json:"stacked_regular,omitempty" doc:"percent is a sum: the client's own monthly pick. Set together with stacked_super (invariant 6 amendment 2026-07-31)"`
	StackedSuper   *string `json:"stacked_super,omitempty" doc:"the барабан granted on top of that pick — mark the row «барабан» when this is set"`
	FriendName     string  `json:"friend_name,omitempty" doc:"карта друга («картой Стаса»); пусто — своя карта. Caps на карте друга не сериализуются никогда"`
	FriendUsername string  `json:"friend_username,omitempty"`
}

func lookupEntryDTO(e LookupEntry) LookupEntryDTO {
	return LookupEntryDTO{
		BankClientID: e.ClientID, Kind: string(e.Kind), RawTitle: e.RawTitle,
		BankName: e.BankName, ClientLabel: e.ClientLabel, HolderLabel: e.HolderLabel, Percent: decToStr(e.Percent),
		CurrencyKind: string(e.CurrencyKind), PointsLabel: e.PointsLabel,
		CapValue: decToStr(e.CapValue), CapPerCategory: decToStr(e.CapPerCategory),
		CapScope: string(e.CapScope), OfferCapValue: decToStr(e.OfferCapValue),
		PeriodStart:    e.Period.Start.Format("2006-01-02"),
		PeriodEnd:      e.Period.End.Format("2006-01-02"),
		StackedRegular: decToStr(e.StackedRegular), StackedSuper: decToStr(e.StackedSuper),
		FriendName: e.FriendName, FriendUsername: e.FriendUsername,
	}
}

// AvailableEntryDTO is one S3b «Можно выбрать» row: an offered-but-unselected
// menu offer with a fact-based verdict (spec S3b, 2026-07-16).
type AvailableEntryDTO struct {
	LookupEntryDTO
	OfferID    int64  `json:"offer_id" doc:"category_offer id — «Отметить выбранной» posts the ordinary selection for it"`
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
	// MinAmount bounds the qualifying purchase («при заказе от 2 000 ₽»);
	// CapValue bounds the payout. Independent — an offer may carry either.
	MinAmount     *string     `json:"min_amount,omitempty"`
	Notes         *string     `json:"notes,omitempty"`
	AttachmentIDs []uuid.UUID `json:"attachment_ids,omitempty"`
}

// partnerOfferDTO maps the shared columns. Callers add BankName and
// AttachmentIDs, which come from joins the write queries do not return.
func partnerOfferDTO(p db.PartnerOffer) PartnerOfferDTO {
	return PartnerOfferDTO{
		ID: p.ID, BankID: p.BankID, BankClientID: p.BankClientID,
		MerchantTitle: p.MerchantTitle, Percent: decToStr(p.Percent),
		ValidFrom: fmtDatePtr(p.ValidFrom), ValidTo: fmtDatePtr(p.ValidTo),
		CapValue: decToStr(p.CapValue), MinAmount: decToStr(p.MinAmount),
		Notes: p.Notes,
	}
}

// partnerFields is the parsed form of the four free-text/date inputs that
// create and update share verbatim.
type partnerFields struct {
	percent, cap, min *decimal.Decimal
	from, to          *time.Time
}

func parsePartnerFields(percent, capValue, minAmount, validFrom, validTo *string) (partnerFields, error) {
	var f partnerFields
	var err error
	if f.percent, err = strToDec(percent, "percent"); err != nil {
		return f, err
	}
	if f.cap, err = strToDec(capValue, "cap_value"); err != nil {
		return f, err
	}
	if f.min, err = strToDec(minAmount, "min_amount"); err != nil {
		return f, err
	}
	if validFrom != nil {
		t, err := parseDate(*validFrom, "valid_from")
		if err != nil {
			return f, err
		}
		f.from = &t
	}
	if validTo != nil {
		t, err := parseDate(*validTo, "valid_to")
		if err != nil {
			return f, err
		}
		f.to = &t
	}
	return f, nil
}

// attachToPartner links screenshots after checking each one belongs to the
// caller — an attachment id is guessable, so ownership is verified per file.
func (s *Service) attachToPartner(ctx context.Context, userID uuid.UUID, offerID int64, ids []uuid.UUID) error {
	for _, aid := range ids {
		if _, err := s.Q.GetAttachmentForUser(ctx, db.GetAttachmentForUserParams{ID: aid, UserID: &userID}); err != nil {
			return httpErr(notFound(err))
		}
		if err := s.Q.AttachToPartnerOffer(ctx, db.AttachToPartnerOfferParams{
			PartnerOfferID: offerID, AttachmentID: aid,
		}); err != nil {
			return err
		}
	}
	return nil
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
	Emoji       string         `json:"emoji,omitempty" doc:"canonical category icon for the list"`
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
	Emoji       string         `json:"emoji,omitempty" doc:"all-purchases icon — keeps the list's icon column aligned"`
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
	MidPeriodAdd   string                `json:"mid_period_add,omitempty" enum:"allowed,locked_after_first,paid,unknown" doc:"can a category still be ADDED to a live period"`
	Activation     string                `json:"activation,omitempty" enum:"immediate,next_day,unknown" doc:"next_day (МКБ): a fresh pick won't cover a purchase made right now"`
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
	// --- programs / tiers (seed-managed reference data, read-only here) ---

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

	// Canonical categories are seed-managed and read-only over the API: they
	// are the cross-bank identity every account's lookup and overview key on,
	// and the taxonomy already covers the banks' menus, so a new one is rare
	// enough to belong in the knowledge base and the seed derived from it. A
	// category the seed does not know yet is recorded as a canonical-less
	// catalog row, which is what that state is for.

	// --- bank picker catalogs ---

	huma.Register(api, huma.Operation{
		OperationID: "cashback-bank-category-list", Method: http.MethodGet,
		Path: "/api/v1/cashback/banks/{bank_id}/categories", Summary: "The bank's picker catalog (current menu rows)", Tags: []string{"cashback"},
	}, func(ctx context.Context, in *struct {
		BankID int32 `path:"bank_id"`
	}) (*struct{ Body []BankCategoryDTO }, error) {
		rows, err := s.ListBankCategories(ctx, auth.UserID(ctx), in.BankID)
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
		Path: "/api/v1/cashback/bank-categories", Summary: "Add a custom category to a bank's picker catalog (visible to its author only)", Tags: []string{"cashback"},
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
		bc, err := s.CreateBankCategory(ctx, auth.UserID(ctx), in.Body.BankID, in.Body.Title, in.Body.CanonicalCategoryID, kind, in.Body.Emoji)
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
				Percent: decToStr(o.Percent), CapValue: decToStr(o.CapValue),
				Kind: string(o.Kind), Notes: o.Notes, SelectionID: o.SelectionID,
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
			CapValue            *string `json:"cap_value,omitempty" doc:"per-offer КБ cap for the period (ВТБ «Кешбэк до N ₽»); static display, no tracking"`
		}
	}) (*struct{ Body CategoryOfferDTO }, error) {
		pctVal, err := strToDec(in.Body.Percent, "percent")
		if err != nil {
			return nil, err
		}
		capVal, err := strToDec(in.Body.CapValue, "cap_value")
		if err != nil {
			return nil, err
		}
		kind := OfferKind(in.Body.Kind)
		if kind == "" {
			kind = OfferRegular
		}
		o, err := s.CreateCategoryOffer(ctx, auth.UserID(ctx), in.Body.OfferPeriodID, in.Body.RawTitle, in.Body.CanonicalCategoryID, pctVal, kind, in.Body.Notes, in.Body.BankCategoryID, capVal)
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body CategoryOfferDTO }{CategoryOfferDTO{
			ID: o.ID, OfferPeriodID: o.OfferPeriodID, RawTitle: o.RawTitle,
			CanonicalCategoryID: o.CanonicalCategoryID, BankCategoryID: o.BankCategoryID,
			Percent: decToStr(o.Percent), CapValue: decToStr(o.CapValue),
			Kind: string(o.Kind), Notes: o.Notes,
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
		capVal, err := strToDec(in.Body.CapValue, "cap_value")
		if err != nil {
			return nil, err
		}
		kind := OfferKind(in.Body.Kind)
		if kind == "" {
			kind = OfferRegular
		}
		o, err := s.UpdateCategoryOffer(ctx, auth.UserID(ctx), in.ID, in.Body.RawTitle, in.Body.CanonicalCategoryID, pctVal, kind, in.Body.Notes, in.Body.BankCategoryID, capVal)
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body CategoryOfferDTO }{CategoryOfferDTO{
			ID: o.ID, OfferPeriodID: o.OfferPeriodID, RawTitle: o.RawTitle,
			CanonicalCategoryID: o.CanonicalCategoryID, BankCategoryID: o.BankCategoryID,
			Percent: decToStr(o.Percent), CapValue: decToStr(o.CapValue),
			Kind: string(o.Kind), Notes: o.Notes,
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
			out.Body.Base = &OverviewBaseDTO{Emoji: res.Base.Emoji, Best: lookupEntryDTO(res.Base.Best), OthersCount: res.Base.OthersCount}
		}
		out.Body.Categories = make([]OverviewCategoryDTO, len(res.Categories))
		for i, g := range res.Categories {
			out.Body.Categories[i] = OverviewCategoryDTO{
				CategoryID: g.CategoryID, Slug: g.Slug, TitleRu: g.TitleRu, Emoji: g.Emoji,
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
				PointsLabel: c.PointsLabel, MidPeriodAdd: c.MidPeriodAdd, Activation: c.Activation,
				PeriodID: c.PeriodID, SlotsUsed: c.SlotsUsed, MaxCategories: c.MaxCategories,
			}
			dto.Cards = make([]OverviewCardChipDTO, len(c.Cards))
			for j, cc := range c.Cards {
				dto.Cards[j] = OverviewCardChipDTO(cc)
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
			Ranked    []LookupEntryDTO     `json:"ranked" doc:"regular + super + special, marked by kind (invariant 6 amendment 2026-07-27)"`
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
				Ranked    []LookupEntryDTO     `json:"ranked" doc:"regular + super + special, marked by kind (invariant 6 amendment 2026-07-27)"`
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
		for _, e := range res.Fallback {
			out.Body.Fallback = append(out.Body.Fallback, lookupEntryDTO(e))
		}
		for _, a := range res.Available {
			out.Body.Available = append(out.Body.Available, AvailableEntryDTO{
				LookupEntryDTO: lookupEntryDTO(a.Entry),
				OfferID:        a.OfferID,
				Verdict:        string(a.Verdict), Activation: string(a.Activation),
			})
		}
		for _, p := range res.Partner {
			out.Body.Partner = append(out.Body.Partner, PartnerOfferDTO{
				ID: p.ID, BankID: p.BankID, BankName: p.BankName, BankClientID: p.BankClientID,
				MerchantTitle: p.MerchantTitle, Percent: decToStr(p.Percent),
				ValidFrom: fmtDatePtr(p.ValidFrom), ValidTo: fmtDatePtr(p.ValidTo),
				CapValue: decToStr(p.CapValue), MinAmount: decToStr(p.MinAmount), Notes: p.Notes,
			})
		}
		// The dead-end message only when there is truly nothing — neither
		// ranked nor available (spec S3b).
		if len(out.Body.Ranked) == 0 && len(out.Body.Available) == 0 {
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
			MinAmount     *string     `json:"min_amount,omitempty" doc:"minimum qualifying purchase («от 2 000 ₽»); display only"`
			Notes         *string     `json:"notes,omitempty"`
			AttachmentIDs []uuid.UUID `json:"attachment_ids,omitempty"`
		}
	}) (*struct{ Body PartnerOfferDTO }, error) {
		f, err := parsePartnerFields(in.Body.Percent, in.Body.CapValue, in.Body.MinAmount,
			in.Body.ValidFrom, in.Body.ValidTo)
		if err != nil {
			return nil, err
		}
		if err := s.AssertOwnsClient(ctx, auth.UserID(ctx), in.Body.BankClientID); err != nil {
			return nil, httpErr(err)
		}
		p, err := s.Q.CreatePartnerOffer(ctx, db.CreatePartnerOfferParams{
			UserID: auth.UserID(ctx), BankID: in.Body.BankID, BankClientID: in.Body.BankClientID,
			MerchantTitle: in.Body.MerchantTitle, Percent: f.percent,
			ValidFrom: f.from, ValidTo: f.to, CapValue: f.cap, MinAmount: f.min,
			Notes: in.Body.Notes,
		})
		if err != nil {
			return nil, httpErr(err)
		}
		if err := s.attachToPartner(ctx, auth.UserID(ctx), p.ID, in.Body.AttachmentIDs); err != nil {
			return nil, err
		}
		out := partnerOfferDTO(p)
		out.AttachmentIDs = in.Body.AttachmentIDs
		return &struct{ Body PartnerOfferDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-partner-offer-get", Method: http.MethodGet,
		Path: "/api/v1/cashback/partner-offers/{id}", Summary: "One partner offer with its screenshots", Tags: []string{"cashback"},
	}, func(ctx context.Context, in *struct {
		ID int64 `path:"id"`
	}) (*struct{ Body PartnerOfferDTO }, error) {
		row, err := s.Q.GetPartnerOfferForUser(ctx, db.GetPartnerOfferForUserParams{
			ID: in.ID, UserID: auth.UserID(ctx),
		})
		if err != nil {
			return nil, httpErr(notFound(err))
		}
		out := PartnerOfferDTO{
			ID: row.ID, BankID: row.BankID, BankName: row.BankName, BankClientID: row.BankClientID,
			MerchantTitle: row.MerchantTitle, Percent: decToStr(row.Percent),
			ValidFrom: fmtDatePtr(row.ValidFrom), ValidTo: fmtDatePtr(row.ValidTo),
			CapValue: decToStr(row.CapValue), MinAmount: decToStr(row.MinAmount), Notes: row.Notes,
		}
		atts, err := s.Q.ListPartnerOfferAttachments(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		out.AttachmentIDs = make([]uuid.UUID, len(atts))
		for i, a := range atts {
			out.AttachmentIDs[i] = a.ID
		}
		return &struct{ Body PartnerOfferDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-partner-offer-update", Method: http.MethodPut,
		Path: "/api/v1/cashback/partner-offers/{id}", Summary: "Correct a recorded partner offer", Tags: []string{"cashback"},
	}, func(ctx context.Context, in *struct {
		ID   int64 `path:"id"`
		Body struct {
			BankID        int32   `json:"bank_id"`
			BankClientID  *int64  `json:"bank_client_id,omitempty"`
			MerchantTitle string  `json:"merchant_title" minLength:"1"`
			Percent       *string `json:"percent,omitempty"`
			ValidFrom     *string `json:"valid_from,omitempty" format:"date"`
			ValidTo       *string `json:"valid_to,omitempty" format:"date"`
			CapValue      *string `json:"cap_value,omitempty"`
			MinAmount     *string `json:"min_amount,omitempty"`
			Notes         *string `json:"notes,omitempty"`
		}
	}) (*struct{ Body PartnerOfferDTO }, error) {
		f, err := parsePartnerFields(in.Body.Percent, in.Body.CapValue, in.Body.MinAmount,
			in.Body.ValidFrom, in.Body.ValidTo)
		if err != nil {
			return nil, err
		}
		if err := s.AssertOwnsClient(ctx, auth.UserID(ctx), in.Body.BankClientID); err != nil {
			return nil, httpErr(err)
		}
		p, err := s.Q.UpdatePartnerOfferForUser(ctx, db.UpdatePartnerOfferForUserParams{
			ID: in.ID, UserID: auth.UserID(ctx),
			BankID: in.Body.BankID, BankClientID: in.Body.BankClientID,
			MerchantTitle: in.Body.MerchantTitle, Percent: f.percent,
			ValidFrom: f.from, ValidTo: f.to, CapValue: f.cap, MinAmount: f.min,
			Notes: in.Body.Notes,
		})
		if err != nil {
			return nil, httpErr(notFound(err))
		}
		return &struct{ Body PartnerOfferDTO }{partnerOfferDTO(p)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-partner-offer-attach", Method: http.MethodPost,
		Path: "/api/v1/cashback/partner-offers/{id}/attachments", Summary: "Attach a screenshot", Tags: []string{"cashback"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID   int64 `path:"id"`
		Body struct {
			AttachmentID uuid.UUID `json:"attachment_id"`
		}
	}) (*struct{}, error) {
		userID := auth.UserID(ctx)
		if _, err := s.Q.GetPartnerOfferForUser(ctx, db.GetPartnerOfferForUserParams{ID: in.ID, UserID: userID}); err != nil {
			return nil, httpErr(notFound(err))
		}
		if err := s.attachToPartner(ctx, userID, in.ID, []uuid.UUID{in.Body.AttachmentID}); err != nil {
			return nil, err
		}
		return nil, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-partner-offer-detach", Method: http.MethodDelete,
		Path: "/api/v1/cashback/partner-offers/{id}/attachments/{attachment_id}", Summary: "Remove a screenshot", Tags: []string{"cashback"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID           int64     `path:"id"`
		AttachmentID uuid.UUID `path:"attachment_id"`
	}) (*struct{}, error) {
		if err := s.DetachPartnerScreenshot(ctx, auth.UserID(ctx), in.ID, in.AttachmentID); err != nil {
			return nil, httpErr(err)
		}
		return nil, nil
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
				CapValue: decToStr(p.CapValue), MinAmount: decToStr(p.MinAmount), Notes: p.Notes,
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
		if err := s.DeletePartnerOffer(ctx, auth.UserID(ctx), in.ID); err != nil {
			return nil, httpErr(err)
		}
		return &struct{}{}, nil
	})

	// --- Screenshot recognizer (docs/specs/cashback-recognizer.md): the
	// job extracts a prefill draft; commit replays the four existing
	// write endpoints. 202 = accepted for background processing (~2.5
	// min/screenshot on the reference model) — poll the GET below. ---

	huma.Register(api, huma.Operation{
		OperationID: "cashback-recognition-create", Method: http.MethodPost,
		Path: "/api/v1/cashback/recognitions", Summary: "Start recognizing uploaded picker screenshots", Tags: []string{"cashback"},
		DefaultStatus: http.StatusAccepted,
	}, func(ctx context.Context, in *struct {
		Body struct {
			BankClientID  int64       `json:"bank_client_id"`
			AttachmentIDs []uuid.UUID `json:"attachment_ids" minItems:"1" maxItems:"10"`
		}
	}) (*struct{ Body RecognitionJobDTO }, error) {
		job, err := s.StartRecognition(ctx, auth.UserID(ctx), in.Body.BankClientID, in.Body.AttachmentIDs)
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body RecognitionJobDTO }{job}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-recognition-get", Method: http.MethodGet,
		Path: "/api/v1/cashback/recognitions/{id}", Summary: "Poll a recognition job", Tags: []string{"cashback"},
	}, func(ctx context.Context, in *struct {
		ID uuid.UUID `path:"id"`
	}) (*struct{ Body RecognitionJobDTO }, error) {
		job, err := s.GetRecognition(auth.UserID(ctx), in.ID)
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body RecognitionJobDTO }{job}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cashback-friends", Method: http.MethodGet,
		Path: "/api/v1/cashback/friends", Summary: "Кешбек друзей: shared clients' window (current + next month)", Tags: []string{"cashback"},
		// No date parameter on purpose (invariant 8): the shared window is
		// [today .. end of next month] from SERVER time — a caller-supplied
		// date must never page a friend's history.
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			Friends []FriendCashbackDTO `json:"friends"`
		}
	}, error) {
		views, err := s.FriendsOffers(ctx, auth.UserID(ctx), time.Now())
		if err != nil {
			return nil, httpErr(err)
		}
		out := &struct {
			Body struct {
				Friends []FriendCashbackDTO `json:"friends"`
			}
		}{}
		out.Body.Friends = make([]FriendCashbackDTO, len(views))
		for i, v := range views {
			out.Body.Friends[i] = friendCashbackDTO(v)
		}
		return out, nil
	})
}

// FriendOfferDTO is one row of a friend's shared picture. Deliberately no
// cap/limit fields — «лимиты не передаются» is a published-policy statement
// (friends-sharing invariant 4), enforced by shape, not filtering.
type FriendOfferDTO struct {
	RawTitle     string  `json:"raw_title"`
	Percent      *string `json:"percent,omitempty"`
	Kind         string  `json:"kind"` // regular | super | special — the UI golds барабан/спец
	CurrencyKind string  `json:"currency_kind"`
	PointsLabel  string  `json:"points_label,omitempty"`
}

// FriendPeriodDTO is one period inside the shared window — the current one
// and anything reaching into next month (invariant 8), earliest first.
type FriendPeriodDTO struct {
	PeriodStart string           `json:"period_start"`
	PeriodEnd   string           `json:"period_end"`
	Selected    []FriendOfferDTO `json:"selected"`
	Granted     []FriendOfferDTO `json:"granted" doc:"барабан/спец — granted, not chosen"`
	Menu        []FriendOfferDTO `json:"menu" doc:"unselected rows — «ты можешь выбрать X»"`
}

type FriendSharedClientDTO struct {
	BankClientID int64             `json:"bank_client_id"`
	BankName     string            `json:"bank_name"`
	HolderLabel  string            `json:"holder_label,omitempty"`
	Periods      []FriendPeriodDTO `json:"periods" doc:"periods in the shared window [today .. конец следующего месяца]; empty — ничего не внесено"`
}

type FriendCashbackDTO struct {
	UserID      uuid.UUID               `json:"user_id"`
	Username    string                  `json:"username"`
	DisplayName string                  `json:"display_name"`
	Clients     []FriendSharedClientDTO `json:"clients"`
}

func friendCashbackDTO(v FriendView) FriendCashbackDTO {
	out := FriendCashbackDTO{
		UserID: v.UserID, Username: v.Username, DisplayName: v.DisplayName,
		Clients: make([]FriendSharedClientDTO, len(v.Clients)),
	}
	offerDTOs := func(rows []FriendOfferView) []FriendOfferDTO {
		dtos := make([]FriendOfferDTO, len(rows))
		for i, r := range rows {
			dtos[i] = FriendOfferDTO{
				RawTitle: r.RawTitle, Percent: decToStr(r.Percent), Kind: string(r.Kind),
				CurrencyKind: string(r.CurrencyKind), PointsLabel: r.PointsLabel,
			}
		}
		return dtos
	}
	for i, c := range v.Clients {
		dto := FriendSharedClientDTO{
			BankClientID: c.BankClientID, BankName: c.BankName, HolderLabel: c.HolderLabel,
			Periods: make([]FriendPeriodDTO, len(c.Periods)),
		}
		for j, p := range c.Periods {
			dto.Periods[j] = FriendPeriodDTO{
				PeriodStart: p.Period.Start.Format("2006-01-02"),
				PeriodEnd:   p.Period.End.Format("2006-01-02"),
				Selected:    offerDTOs(p.Selected), Granted: offerDTOs(p.Granted), Menu: offerDTOs(p.Menu),
			}
		}
		out.Clients[i] = dto
	}
	return out
}
