package mcc

import (
	"testing"
	"time"
)

func rec(fields ...string) []string { return fields }

func TestParsePosRow(t *testing.T) {
	full := rec("f9d42d89-4377-2a7d-2e1c-f8788e4182be", "Ветклиника Джунгли", "AVRORA",
		"742", "offline", "Сочи, ул. Голубая 1", "2", "2019-09-30", "2021-02-26")
	row, err := parsePosRow(full)
	if err != nil {
		t.Fatalf("parsePosRow(full) error: %v", err)
	}
	if row.Name != "Ветклиника Джунгли" || row.MCC != 742 || row.Confirmations != 2 {
		t.Errorf("parsePosRow(full) = %+v", row)
	}
	if row.MerchantTitle == nil || *row.MerchantTitle != "AVRORA" {
		t.Errorf("merchant_title = %v, want AVRORA", row.MerchantTitle)
	}
	if row.Type == nil || *row.Type != "offline" {
		t.Errorf("type = %v, want offline", row.Type)
	}
	if !row.CreatedAt.Equal(time.Date(2019, 9, 30, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("created_at = %v", row.CreatedAt)
	}
	if row.LastConfirmedAt == nil || !row.LastConfirmedAt.Equal(time.Date(2021, 2, 26, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("last_confirmed_at = %v", row.LastConfirmedAt)
	}

	sparse := rec("ad17060c-d767-764a-9add-55ad39db30f9", "ДомВет", "",
		"742", "", "", "0", "2017-03-30", "")
	row, err = parsePosRow(sparse)
	if err != nil {
		t.Fatalf("parsePosRow(sparse) error: %v", err)
	}
	if row.MerchantTitle != nil || row.Type != nil || row.Address != nil || row.LastConfirmedAt != nil {
		t.Errorf("sparse row: empty fields must map to nil, got %+v", row)
	}

	bad := map[string][]string{
		"field count":   rec("ad17060c-d767-764a-9add-55ad39db30f9", "X", "", "742"),
		"uuid":          rec("not-a-uuid", "X", "", "742", "", "", "0", "2017-03-30", ""),
		"empty title":   rec("ad17060c-d767-764a-9add-55ad39db30f9", "", "", "742", "", "", "0", "2017-03-30", ""),
		"mcc":           rec("ad17060c-d767-764a-9add-55ad39db30f9", "X", "", "abc", "", "", "0", "2017-03-30", ""),
		"type":          rec("ad17060c-d767-764a-9add-55ad39db30f9", "X", "", "742", "shop", "", "0", "2017-03-30", ""),
		"confirmations": rec("ad17060c-d767-764a-9add-55ad39db30f9", "X", "", "742", "", "", "many", "2017-03-30", ""),
		"created_at":    rec("ad17060c-d767-764a-9add-55ad39db30f9", "X", "", "742", "", "", "0", "30.03.2017", ""),
		"actual_at":     rec("ad17060c-d767-764a-9add-55ad39db30f9", "X", "", "742", "", "", "0", "2017-03-30", "yesterday"),
	}
	for name, fields := range bad {
		if _, err := parsePosRow(fields); err == nil {
			t.Errorf("parsePosRow(bad %s): expected error", name)
		}
	}
}
