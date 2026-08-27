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
		rows, err := s.SearchMerchants(ctx, in.Query, in.Limit)
		if err != nil {
			return nil, err
		}
		out := make([]MerchantDTO, len(rows))
		for i, r := range rows {
			d := MerchantDTO{
				ID: r.ID.String(), Name: r.Name, MerchantTitle: r.MerchantTitle,
				Address: r.Address, LastConfirmedAt: r.LastConfirmedAt,
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
		entry, rows, err := s.Resolve(ctx, auth.UserID(ctx), code)
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
