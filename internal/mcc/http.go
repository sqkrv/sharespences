package mcc

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/sqkrv/sharespences/internal/auth"
	"github.com/sqkrv/sharespences/internal/db"
)

// CodeDTO is one dictionary entry; Code is zero-padded («0742») — the form
// banks print.
type CodeDTO struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

func codeDTO(m db.Mcc) CodeDTO {
	return CodeDTO{Code: FormatCode(m.Code), Name: m.Name, Description: m.Description}
}

// ResolveEntryDTO — one bank's catalog category containing the code.
type ResolveEntryDTO struct {
	BankID         int32   `json:"bank_id"`
	BankName       string  `json:"bank_name"`
	BankColorHex   *string `json:"bank_color_hex,omitempty"`
	BankCategoryID int64   `json:"bank_category_id"`
	Title          string  `json:"title"`
	Kind           string  `json:"kind"` // regular | super | special — special pays only in its channel
	Emoji          *string `json:"emoji,omitempty"`
	CanonicalSlug  *string `json:"canonical_slug,omitempty"`
	CanonicalTitle *string `json:"canonical_title,omitempty"`
	Note           *string `json:"note,omitempty"`
}

// CanonicalRefDTO — a distinct canonical category among the resolutions;
// feeds the existing cashback category lookup («Какой картой?»).
type CanonicalRefDTO struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

// MerchantDTO — one point of sale from the imported merchant base
// (mcc-codes.ru scrape; данные mcc-codes.ru — the SPA renders the credit).
type MerchantDTO struct {
	ID              string     `json:"id"` // the site's own row UUID — stable across re-imports
	Name            string     `json:"name"`
	MerchantTitle   *string    `json:"merchant_title,omitempty"`
	MCC             string     `json:"mcc"` // zero-padded
	Type            *string    `json:"type,omitempty" enum:"offline,online,app,other"`
	Address         *string    `json:"address,omitempty"`
	Confirmations   int64      `json:"confirmations"`
	LastConfirmedAt *time.Time `json:"last_confirmed_at,omitempty"`
	Status          string     `json:"status,omitempty" enum:"approved,pending" doc:"pending rows are the caller's own submissions awaiting moderation"`
}

// SimilarPointDTO is one 5e duplicate-net hit: an existing точка the form
// offers to open instead of creating a copy.
type SimilarPointDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	MCC  string `json:"mcc"`
}

type ChangeDTO struct {
	ID             int64     `json:"id"`
	BankID         int32     `json:"bank_id"`
	BankName       string    `json:"bank_name"`
	BankCategoryID *int64    `json:"bank_category_id,omitempty"`
	CategoryTitle  string    `json:"category_title"`
	MCCCode        *string   `json:"mcc_code,omitempty"` // padded; null for category_* events
	Action         string    `json:"action" enum:"imported,added,removed,category_added,category_removed"`
	NotedAt        time.Time `json:"noted_at"`
	Source         string    `json:"source"`
	Note           *string   `json:"note,omitempty"`
}

func httpErr(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return huma.Error404NotFound(ErrNotFound.Error())
	case errors.Is(err, ErrBadCode):
		return huma.Error422UnprocessableEntity(ErrBadCode.Error())
	}
	return err
}

// RegisterHTTP mounts the MCC module's API (session-guarded like the rest
// of /api/v1).
func RegisterHTTP(api huma.API, s *Service) {
	huma.Register(api, huma.Operation{
		OperationID: "mcc-code-search", Method: http.MethodGet,
		Path: "/api/v1/mcc/codes", Summary: "Search the MCC dictionary (code prefix or name substring)", Tags: []string{"mcc"},
	}, func(ctx context.Context, in *struct {
		Query string `query:"query" required:"true" minLength:"1"`
		Limit int32  `query:"limit" default:"20" minimum:"1" maximum:"50"`
	}) (*struct{ Body []CodeDTO }, error) {
		rows, err := s.Search(ctx, in.Query, in.Limit)
		if err != nil {
			return nil, err
		}
		out := make([]CodeDTO, len(rows))
		for i, r := range rows {
			out[i] = codeDTO(r)
		}
		return &struct{ Body []CodeDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "mcc-merchant-search", Method: http.MethodGet,
		Path: "/api/v1/mcc/merchants", Summary: "Search the merchant base (points of sale) by name", Tags: []string{"mcc"},
	}, func(ctx context.Context, in *struct {
		Query string `query:"query" required:"true" minLength:"2"`
		Limit int32  `query:"limit" default:"20" minimum:"1" maximum:"50"`
	}) (*struct{ Body []MerchantDTO }, error) {
		rows, err := s.SearchMerchants(ctx, auth.UserID(ctx), in.Query, in.Limit)
		if err != nil {
			return nil, err
		}
		out := make([]MerchantDTO, len(rows))
		for i, r := range rows {
			d := MerchantDTO{
				ID: r.ID.String(), Name: r.Name, MerchantTitle: r.MerchantTitle,
				Address: r.Address, LastConfirmedAt: r.LastConfirmedAt,
				Status: string(r.Status),
			}
			if r.MccCode != nil {
				d.MCC = FormatCode(*r.MccCode)
			}
			if r.PosType != "" {
				t := r.PosType
				d.Type = &t
			}
			if r.Confirmations != nil {
				d.Confirmations = *r.Confirmations
			}
			out[i] = d
		}
		return &struct{ Body []MerchantDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "mcc-pos-similar", Method: http.MethodGet,
		Path: "/api/v1/mcc/points-of-sale/similar", Summary: "Existing points that look like the one being created", Tags: []string{"mcc"},
	}, func(ctx context.Context, in *struct {
		Code string `query:"mcc" required:"true" pattern:"^[0-9]{3,4}$"`
		Name string `query:"name" required:"true" minLength:"2"`
	}) (*struct{ Body []SimilarPointDTO }, error) {
		code, err := ParseCode(in.Code)
		if err != nil {
			return nil, httpErr(err)
		}
		rows, err := s.SimilarPoints(ctx, auth.UserID(ctx), code, in.Name)
		if err != nil {
			return nil, err
		}
		out := make([]SimilarPointDTO, len(rows))
		for i, r := range rows {
			out[i] = SimilarPointDTO{ID: r.ID.String(), Name: r.Name}
			if r.MccCode != nil {
				out[i].MCC = FormatCode(*r.MccCode)
			}
		}
		return &struct{ Body []SimilarPointDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "mcc-pos-create", Method: http.MethodPost,
		Path: "/api/v1/mcc/points-of-sale", Summary: "Add a точка продаж (pending until moderated)", Tags: []string{"mcc"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Body struct {
			MCC           string  `json:"mcc" pattern:"^[0-9]{3,4}$" doc:"4 цифры — из истории транзакций"`
			Name          string  `json:"name" minLength:"2" maxLength:"120"`
			MerchantTitle *string `json:"merchant_title,omitempty" maxLength:"120" doc:"как в выписке или SMS, латиницей"`
			Type          string  `json:"type" enum:"offline,online,app,other"`
			Address       *string `json:"address,omitempty" maxLength:"250" doc:"офлайн — адрес, онлайн — сайт, приложение — название, другое — описание"`
		}
	}) (*struct{ Body MerchantDTO }, error) {
		code, err := ParseCode(in.Body.MCC)
		if err != nil {
			return nil, httpErr(err)
		}
		p, err := s.CreatePoint(ctx, auth.UserID(ctx), db.CreateUserPointOfSaleParams{
			Name:          in.Body.Name,
			MerchantTitle: in.Body.MerchantTitle,
			MccCode:       &code,
			Type:          db.NullPointOfSaleType{PointOfSaleType: db.PointOfSaleType(in.Body.Type), Valid: true},
			Address:       in.Body.Address,
		})
		if err != nil {
			return nil, httpErr(err)
		}
		d := MerchantDTO{ID: p.ID.String(), Name: p.Name, MerchantTitle: p.MerchantTitle, Status: string(p.Status)}
		if p.MccCode != nil {
			d.MCC = FormatCode(*p.MccCode)
		}
		return &struct{ Body MerchantDTO }{d}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "mcc-resolve", Method: http.MethodGet,
		Path: "/api/v1/mcc/resolve", Summary: "Which bank category the MCC falls into, per bank", Tags: []string{"mcc"},
	}, func(ctx context.Context, in *struct {
		Code string `query:"code" required:"true" pattern:"^[0-9]{3,4}$"`
	}) (*struct {
		Body struct {
			Code       CodeDTO           `json:"code"`
			Banks      []ResolveEntryDTO `json:"banks"`
			Canonicals []CanonicalRefDTO `json:"canonicals"`
		}
	}, error) {
		code, err := ParseCode(in.Code)
		if err != nil {
			return nil, httpErr(err)
		}
		entry, rows, err := s.Resolve(ctx, code)
		if err != nil {
			return nil, httpErr(err)
		}
		out := &struct {
			Body struct {
				Code       CodeDTO           `json:"code"`
				Banks      []ResolveEntryDTO `json:"banks"`
				Canonicals []CanonicalRefDTO `json:"canonicals"`
			}
		}{}
		out.Body.Code = codeDTO(entry)
		out.Body.Banks = make([]ResolveEntryDTO, len(rows))
		for i, r := range rows {
			emoji := r.BankEmoji
			if emoji == nil {
				emoji = r.CanonicalEmoji
			}
			out.Body.Banks[i] = ResolveEntryDTO{
				BankID: r.BankID, BankName: r.BankName, BankColorHex: r.ColorHex,
				BankCategoryID: r.BankCategoryID, Title: r.Title, Kind: string(r.Kind),
				Emoji: emoji, CanonicalSlug: r.CanonicalSlug, CanonicalTitle: r.CanonicalTitle,
				Note: r.Note,
			}
		}
		for _, c := range DedupCanonicals(rows) {
			out.Body.Canonicals = append(out.Body.Canonicals, CanonicalRefDTO(c))
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "mcc-changes", Method: http.MethodGet,
		Path: "/api/v1/mcc/changes", Summary: "Newest category/MCC rule changes (journal)", Tags: []string{"mcc"},
	}, func(ctx context.Context, in *struct {
		Limit int32 `query:"limit" default:"50" minimum:"1" maximum:"500"`
	}) (*struct{ Body []ChangeDTO }, error) {
		rows, err := s.Changes(ctx, in.Limit)
		if err != nil {
			return nil, err
		}
		out := make([]ChangeDTO, len(rows))
		for i, r := range rows {
			var code *string
			if r.MccCode != nil {
				c := FormatCode(*r.MccCode)
				code = &c
			}
			out[i] = ChangeDTO{
				ID: r.ID, BankID: r.BankID, BankName: r.BankName,
				BankCategoryID: r.BankCategoryID, CategoryTitle: r.CategoryTitle,
				MCCCode: code, Action: string(r.Action), NotedAt: r.NotedAt,
				Source: r.Source, Note: r.Note,
			}
		}
		return &struct{ Body []ChangeDTO }{out}, nil
	})
}
