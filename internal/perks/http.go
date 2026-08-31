package perks

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/sqkrv/sharespences/internal/auth"
	"github.com/sqkrv/sharespences/internal/db"
)

// dateFmt is the wire form of every date in this module. Windows and ledger
// rows are calendar facts — the day the bank compensated a ride, the day a
// window closes — so they travel as plain dates and never acquire a timezone
// on the way.
const dateFmt = "2006-01-02"

func parseDate(s, field string) (time.Time, error) {
	t, err := time.Parse(dateFmt, s)
	if err != nil {
		return time.Time{}, huma.Error422UnprocessableEntity(fmt.Sprintf("%s: ожидается дата вида ГГГГ-ММ-ДД, получено «%s»", field, s))
	}
	return t, nil
}

// optDate reads an optional date field, defaulting to today. The client sends
// one whenever it knows better — recording yesterday's ride today is the
// ordinary case, not the exception.
func optDate(s *string, field string) (time.Time, error) {
	if s == nil || *s == "" {
		return time.Now(), nil
	}
	return parseDate(*s, field)
}

func httpErr(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return huma.Error404NotFound("не найдено")
	case errors.Is(err, ErrPerkHasQuotas), errors.Is(err, ErrPerkExists):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, ErrNameLength), errors.Is(err, ErrUnitLength),
		errors.Is(err, ErrWindowOrder), errors.Is(err, ErrSizeNegative),
		errors.Is(err, ErrNestingTooDeep), errors.Is(err, ErrChildOutsideParen),
		errors.Is(err, ErrChildMismatch), errors.Is(err, ErrSizeLocked),
		errors.Is(err, ErrQtyPositive), errors.Is(err, ErrQtyNonNegative),
		errors.Is(err, ErrQtyNonZero), errors.Is(err, ErrRemainingNegative),
		errors.Is(err, ErrUnknownKind):
		return huma.Error422UnprocessableEntity(err.Error())
	}
	return err
}

// ------------------------------------------------------------------- DTOs

// PerkDTO is a perk definition: what the bank grants, in the user's words.
// A perk belongs to a держатель (00025): «Компенсация такси» at Альфа is one
// row per account, each with its own size, note and history.
type PerkDTO struct {
	ID           int64   `json:"id"`
	BankClientID int64   `json:"bank_client_id"`
	ClientLabel  *string `json:"client_label,omitempty" doc:"держатель («Мама»); null — сам владелец аккаунта"`
	BankID       int32   `json:"bank_id"`
	BankName     string  `json:"bank_name,omitempty"`
	Name         string  `json:"name"`
	Unit         string  `json:"unit" doc:"счётная единица в единственном числе: «поездка», «преференция», «проход»"`
	Note         *string `json:"note,omitempty"`
}

// The wire shape and the service row have the same fields, so the conversion
// is the whole mapping — and the day they diverge it stops compiling here
// rather than dropping a field silently.
func perkDTO(p PerkRow) PerkDTO { return PerkDTO(p) }

// PerkDiscrepancyDTO is the «расходится с банком» badge. It is a state, never an
// error: the bank's counter is authoritative and opaque, so the app shows the
// gap and waits for the user to explain it with an adjust.
type PerkDiscrepancyDTO struct {
	SnapshotID int64  `json:"snapshot_id" doc:"сверка, которая разошлась — последняя по этому периоду"`
	Delta      int    `json:"delta" doc:"счётчик банка минус вычисленный остаток; минус — банк уже списал то, чего нет в журнале"`
	Computed   int    `json:"computed"`
	Bank       int    `json:"bank"`
	ObservedOn string `json:"observed_on" format:"date"`
}

// PerkQuotaDTO is a window with its counters worked out as of the request date.
type PerkQuotaDTO struct {
	ID            int64               `json:"id"`
	ParentQuotaID *int64              `json:"parent_quota_id,omitempty"`
	WindowStart   string              `json:"window_start" format:"date"`
	WindowEnd     string              `json:"window_end" format:"date"`
	InitialSize   int                 `json:"initial_size" doc:"размер, с которым период открылся; дальше его двигают события"`
	Size          int                 `json:"size" doc:"действующий размер: последний resize плюс grant'ы и adjust'ы"`
	Used          int                 `json:"used" doc:"для годового пула включает списания месячных периодов внутри него"`
	Remaining     int                 `json:"remaining" doc:"может быть отрицательным — это расхождение, а не ошибка"`
	Note          *string             `json:"note,omitempty"`
	Discrepancy   *PerkDiscrepancyDTO `json:"discrepancy,omitempty"`
	LastSeenOn    *string             `json:"last_seen_on,omitempty" format:"date" doc:"дата последней сверки со счётчиком банка"`
	Children      []PerkQuotaDTO      `json:"children,omitempty" doc:"месячные подпериоды внутри годового пула"`
}

func quotaDTO(v QuotaView) PerkQuotaDTO {
	out := PerkQuotaDTO{
		ID: v.ID, ParentQuotaID: v.ParentID,
		WindowStart: v.Window.Start.Format(dateFmt), WindowEnd: v.Window.End.Format(dateFmt),
		InitialSize: v.InitialSize, Size: v.Size, Used: v.Used, Remaining: v.Remaining, Note: v.Note,
	}
	if v.Discrepancy != nil {
		out.Discrepancy = &PerkDiscrepancyDTO{
			SnapshotID: v.Discrepancy.SnapshotID,
			Delta:      v.Discrepancy.Delta, Computed: v.Discrepancy.Computed,
			Bank:       v.Discrepancy.Bank, ObservedOn: v.Discrepancy.ObservedOn.Format(dateFmt),
		}
	}
	if v.LastSeenOn != nil {
		s := v.LastSeenOn.Format(dateFmt)
		out.LastSeenOn = &s
	}
	for _, c := range v.Children {
		out.Children = append(out.Children, quotaDTO(c))
	}
	return out
}

// rawQuotaDTO renders a freshly written row, where no counters have been
// computed yet — the client refetches the screen for those.
func rawQuotaDTO(q db.PerkQuotum) PerkQuotaDTO {
	return PerkQuotaDTO{
		ID: q.ID, ParentQuotaID: q.ParentQuotaID,
		WindowStart: q.WindowStart.Format(dateFmt), WindowEnd: q.WindowEnd.Format(dateFmt),
		InitialSize: int(q.Size), Size: int(q.Size), Remaining: int(q.Size), Note: q.Note,
	}
}

// PerkOverviewDTO is one perk's currently running windows on one bank client.
type PerkOverviewDTO struct {
	PerkID int64          `json:"perk_id"`
	Name   string         `json:"name"`
	Unit   string         `json:"unit"`
	Note   *string        `json:"note,omitempty"`
	Quotas []PerkQuotaDTO `json:"quotas"`
}

// PerkClientDTO is one card on PV-01: a bank client and everything running
// on it. The whole family fleet lives under the owning user as держатель
// labels, the same model cashback uses.
type PerkClientDTO struct {
	BankClientID int64             `json:"bank_client_id"`
	Label        *string           `json:"label,omitempty" doc:"держатель («Мама», «Юля»); null — сам владелец аккаунта"`
	BankID       int32             `json:"bank_id"`
	BankName     string            `json:"bank_name"`
	Perks        []PerkOverviewDTO `json:"perks"`
}

// PerkEventDTO is one ledger row.
type PerkEventDTO struct {
	ID      int64   `json:"id"`
	QuotaID int64   `json:"quota_id"`
	Kind    string  `json:"kind" enum:"use,grant,resize,adjust"`
	Qty     int     `json:"qty"`
	Date    string  `json:"event_date" format:"date"`
	Note    *string `json:"note,omitempty"`
}

// PerkSnapshotDTO is one recorded reading of the bank's own counter.
type PerkSnapshotDTO struct {
	ID         int64   `json:"id"`
	QuotaID    int64   `json:"quota_id"`
	ObservedOn string  `json:"observed_on" format:"date"`
	Remaining  int     `json:"remaining"`
	Computed   *int    `json:"computed,omitempty" doc:"остаток по журналу на дату этой сверки — каждая судится своим днём"`
	Note       *string `json:"note,omitempty"`
}

// PerkHistoryQuotaDTO is a window on PV-02, with the client it belongs to named:
// one perk spans the family fleet, so «15 поездок» has to say whose.
type PerkHistoryQuotaDTO struct {
	PerkQuotaDTO
	ClientLabel *string `json:"client_label,omitempty"`
	BankName    string  `json:"bank_name"`
	Active      bool    `json:"active" doc:"период идёт прямо сейчас"`
}

// ------------------------------------------------------------ registration

// RegisterHTTP mounts the Привилегии operations.
func RegisterHTTP(api huma.API, s *Service) {
	huma.Register(api, huma.Operation{
		OperationID: "perks-overview", Method: http.MethodGet,
		Path: "/api/v1/perks/overview", Summary: "Running perk quotas, by bank client (PV-01)", Tags: []string{"perks"},
	}, func(ctx context.Context, in *struct {
		On string `query:"on" format:"date" doc:"дата, на которую считать остатки; по умолчанию сегодня"`
	}) (*struct{ Body []PerkClientDTO }, error) {
		on := time.Now()
		if in.On != "" {
			var err error
			if on, err = parseDate(in.On, "on"); err != nil {
				return nil, err
			}
		}
		clients, err := s.Overview(ctx, auth.UserID(ctx), on)
		if err != nil {
			return nil, httpErr(err)
		}
		out := make([]PerkClientDTO, len(clients))
		for i, c := range clients {
			row := PerkClientDTO{
				BankClientID: c.ClientID, Label: c.Label, BankID: c.BankID, BankName: c.BankName,
				Perks: make([]PerkOverviewDTO, len(c.Perks)),
			}
			for j, p := range c.Perks {
				pr := PerkOverviewDTO{PerkID: p.PerkID, Name: p.Name, Unit: p.Unit, Note: p.Note, Quotas: []PerkQuotaDTO{}}
				for _, q := range p.Quotas {
					pr.Quotas = append(pr.Quotas, quotaDTO(q))
				}
				row.Perks[j] = pr
			}
			out[i] = row
		}
		return &struct{ Body []PerkClientDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "perks-list", Method: http.MethodGet,
		Path: "/api/v1/perks", Summary: "List the user's perk definitions", Tags: []string{"perks"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []PerkDTO }, error) {
		rows, err := s.ListPerks(ctx, auth.UserID(ctx))
		if err != nil {
			return nil, httpErr(err)
		}
		out := make([]PerkDTO, len(rows))
		for i, r := range rows {
			out[i] = perkDTO(r)
		}
		return &struct{ Body []PerkDTO }{out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "perks-create", Method: http.MethodPost,
		Path: "/api/v1/perks", Summary: "Create a perk", Tags: []string{"perks"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Body struct {
			BankClientID int64   `json:"bank_client_id" doc:"держатель, которому принадлежит привилегия"`
			Name         string  `json:"name" minLength:"1"`
			Unit         string  `json:"unit" minLength:"1" doc:"счётная единица: «поездка», «преференция», «проход»"`
			Note         *string `json:"note,omitempty" doc:"свободный текст: лимиты на поездку, как заявить, ссылка на тариф"`
		}
	}) (*struct{ Body PerkDTO }, error) {
		p, err := s.CreatePerk(ctx, auth.UserID(ctx), in.Body.BankClientID, in.Body.Name, in.Body.Unit, in.Body.Note)
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body PerkDTO }{PerkDTO{
			ID: p.ID, BankClientID: p.BankClientID, Name: p.Name, Unit: p.Unit, Note: p.Note,
		}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "perks-update", Method: http.MethodPatch,
		Path: "/api/v1/perks/{id}", Summary: "Rename a perk or edit its note", Tags: []string{"perks"},
	}, func(ctx context.Context, in *struct {
		ID   int64 `path:"id"`
		Body struct {
			Name    *string `json:"name,omitempty"`
			Unit    *string `json:"unit,omitempty"`
			SetNote bool    `json:"set_note,omitempty" doc:"true — записать note как есть (в том числе пустой, чтобы очистить)"`
			Note    *string `json:"note,omitempty"`
		}
	}) (*struct{ Body PerkDTO }, error) {
		p, err := s.UpdatePerk(ctx, auth.UserID(ctx), in.ID, in.Body.Name, in.Body.Unit, in.Body.SetNote, in.Body.Note)
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body PerkDTO }{PerkDTO{
			ID: p.ID, BankClientID: p.BankClientID, Name: p.Name, Unit: p.Unit, Note: p.Note,
		}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "perks-delete", Method: http.MethodDelete,
		Path: "/api/v1/perks/{id}", Summary: "Delete a perk (409 while it has quota windows)", Tags: []string{"perks"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID int64 `path:"id"`
	}) (*struct{}, error) {
		if err := s.DeletePerk(ctx, auth.UserID(ctx), in.ID); err != nil {
			return nil, httpErr(err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "perks-quota-list", Method: http.MethodGet,
		Path: "/api/v1/perks/{id}/quotas", Summary: "A perk's full window history with its ledger (PV-02)", Tags: []string{"perks"},
	}, func(ctx context.Context, in *struct {
		ID int64  `path:"id"`
		On string `query:"on" format:"date" doc:"дата, на которую считать остатки; по умолчанию сегодня"`
	}) (*struct {
		Body struct {
			Perk      PerkDTO               `json:"perk"`
			Quotas    []PerkHistoryQuotaDTO `json:"quotas"`
			Events    []PerkEventDTO        `json:"events"`
			Snapshots []PerkSnapshotDTO     `json:"snapshots"`
		}
	}, error) {
		now := time.Now()
		if in.On != "" {
			var err error
			if now, err = parseDate(in.On, "on"); err != nil {
				return nil, err
			}
		}
		h, err := s.PerkHistory(ctx, auth.UserID(ctx), in.ID, now)
		if err != nil {
			return nil, httpErr(err)
		}
		out := &struct {
			Body struct {
				Perk      PerkDTO               `json:"perk"`
				Quotas    []PerkHistoryQuotaDTO `json:"quotas"`
				Events    []PerkEventDTO        `json:"events"`
				Snapshots []PerkSnapshotDTO     `json:"snapshots"`
			}
		}{}
		out.Body.Perk = perkDTO(h.Perk)
		out.Body.Quotas = make([]PerkHistoryQuotaDTO, len(h.Quotas))
		today := day(now)
		for i, q := range h.Quotas {
			d := quotaDTO(q.QuotaView)
			out.Body.Quotas[i] = PerkHistoryQuotaDTO{
				PerkQuotaDTO: d, ClientLabel: q.ClientLabel, BankName: q.BankName,
				Active: q.Window.Contains(today),
			}
		}
		out.Body.Events = make([]PerkEventDTO, len(h.Events))
		for i, e := range h.Events {
			out.Body.Events[i] = PerkEventDTO{
				ID: e.ID, QuotaID: e.QuotaID, Kind: string(e.Kind), Qty: e.Qty,
				Date: e.Date.Format(dateFmt), Note: e.Note,
			}
		}
		out.Body.Snapshots = make([]PerkSnapshotDTO, len(h.Snapshots))
		for i, sn := range h.Snapshots {
			c := sn.Computed
			out.Body.Snapshots[i] = PerkSnapshotDTO{
				ID: sn.ID, QuotaID: sn.QuotaID, ObservedOn: sn.ObservedOn.Format(dateFmt),
				Remaining: sn.Remaining, Computed: &c, Note: sn.Note,
			}
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "perks-quota-create", Method: http.MethodPost,
		Path: "/api/v1/perks/{id}/quotas", Summary: "Open a quota window (optionally inside a parent)", Tags: []string{"perks"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		ID   int64 `path:"id"`
		Body struct {
			ParentQuotaID *int64  `json:"parent_quota_id,omitempty" doc:"годовой пул, внутрь которого кладётся месячный период"`
			WindowStart   string  `json:"window_start" format:"date"`
			WindowEnd     string  `json:"window_end" format:"date"`
			Size          int     `json:"size" minimum:"0" maximum:"100000" doc:"размер, с которым период открывается; дальше — события resize"`
			Note          *string `json:"note,omitempty"`
		}
	}) (*struct{ Body PerkQuotaDTO }, error) {
		start, err := parseDate(in.Body.WindowStart, "window_start")
		if err != nil {
			return nil, err
		}
		end, err := parseDate(in.Body.WindowEnd, "window_end")
		if err != nil {
			return nil, err
		}
		q, err := s.CreateQuota(ctx, auth.UserID(ctx), in.ID, in.Body.ParentQuotaID,
			Window{Start: start, End: end}, in.Body.Size, in.Body.Note)
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body PerkQuotaDTO }{rawQuotaDTO(q)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "perks-quota-update", Method: http.MethodPatch,
		Path: "/api/v1/perks/quotas/{id}", Summary: "Edit a window's note (size only before it has history)", Tags: []string{"perks"},
	}, func(ctx context.Context, in *struct {
		ID   int64 `path:"id"`
		Body struct {
			Size    *int    `json:"size,omitempty" minimum:"0" maximum:"100000" doc:"поправка опечатки; после первого события или сверки — 422, размер меняется событием «resize»"`
			SetNote bool    `json:"set_note,omitempty"`
			Note    *string `json:"note,omitempty"`
		}
	}) (*struct{ Body PerkQuotaDTO }, error) {
		q, err := s.UpdateQuota(ctx, auth.UserID(ctx), in.ID, in.Body.Size, in.Body.SetNote, in.Body.Note)
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body PerkQuotaDTO }{rawQuotaDTO(q)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "perks-quota-delete", Method: http.MethodDelete,
		Path: "/api/v1/perks/quotas/{id}", Summary: "Delete a window with its ledger and child windows", Tags: []string{"perks"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID int64 `path:"id"`
	}) (*struct{}, error) {
		if err := s.DeleteQuota(ctx, auth.UserID(ctx), in.ID); err != nil {
			return nil, httpErr(err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "perks-event-create", Method: http.MethodPost,
		Path: "/api/v1/perks/quotas/{id}/events", Summary: "Record a use, grant, resize or adjust", Tags: []string{"perks"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		ID   int64 `path:"id"`
		Body struct {
			Kind string  `json:"kind" enum:"use,grant,resize,adjust" doc:"use — списание (жжёт и месяц, и годовой пул); grant — внеплановая выдача; resize — новый АБСОЛЮТНЫЙ размер; adjust — знаковая сверка"`
			Qty  int     `json:"qty" minimum:"-100000" maximum:"100000"`
			Date *string `json:"event_date,omitempty" format:"date" doc:"по умолчанию сегодня"`
			Note *string `json:"note,omitempty"`
		}
	}) (*struct{ Body PerkEventDTO }, error) {
		on, err := optDate(in.Body.Date, "event_date")
		if err != nil {
			return nil, err
		}
		e, err := s.CreateEvent(ctx, auth.UserID(ctx), in.ID, Kind(in.Body.Kind), in.Body.Qty, on, in.Body.Note)
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body PerkEventDTO }{PerkEventDTO{
			ID: e.ID, QuotaID: e.QuotaID, Kind: string(e.Kind), Qty: int(e.Qty),
			Date: e.EventDate.Format(dateFmt), Note: e.Note,
		}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "perks-event-delete", Method: http.MethodDelete,
		Path: "/api/v1/perks/events/{id}", Summary: "Undo a ledger row", Tags: []string{"perks"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID int64 `path:"id"`
	}) (*struct{}, error) {
		if err := s.DeleteEvent(ctx, auth.UserID(ctx), in.ID); err != nil {
			return nil, httpErr(err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "perks-snapshot-create", Method: http.MethodPost,
		Path: "/api/v1/perks/quotas/{id}/snapshots", Summary: "Record what the bank's counter showed («Сверить»)", Tags: []string{"perks"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		ID   int64 `path:"id"`
		Body struct {
			Remaining  int     `json:"remaining" minimum:"0" maximum:"100000"`
			ObservedOn *string `json:"observed_on,omitempty" format:"date" doc:"по умолчанию сегодня"`
			Note       *string `json:"note,omitempty"`
		}
	}) (*struct{ Body PerkSnapshotDTO }, error) {
		on, err := optDate(in.Body.ObservedOn, "observed_on")
		if err != nil {
			return nil, err
		}
		snap, err := s.CreateSnapshot(ctx, auth.UserID(ctx), in.ID, on, in.Body.Remaining, in.Body.Note)
		if err != nil {
			return nil, httpErr(err)
		}
		return &struct{ Body PerkSnapshotDTO }{PerkSnapshotDTO{
			ID: snap.ID, QuotaID: snap.QuotaID, ObservedOn: snap.ObservedOn.Format(dateFmt),
			Remaining: int(snap.Remaining), Note: snap.Note,
		}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "perks-snapshot-delete", Method: http.MethodDelete,
		Path: "/api/v1/perks/snapshots/{id}", Summary: "Delete a recorded reading", Tags: []string{"perks"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		ID int64 `path:"id"`
	}) (*struct{}, error) {
		if err := s.DeleteSnapshot(ctx, auth.UserID(ctx), in.ID); err != nil {
			return nil, httpErr(err)
		}
		return &struct{}{}, nil
	})
}
