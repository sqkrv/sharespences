package seed

// Coverage sets for the admin sidecar (ADR-0008): «if seed writes it, the
// panel only reads it». Both sets are parsed from the same embedded CSVs
// seedMCC writes from, so the guard can never drift from the behavior.

import (
	"fmt"
	"strconv"
)

// SeededMCCCodes returns the dictionary codes seedMCC unconditionally
// upserts — an edit to one of these is reverted on the next deploy.
func SeededMCCCodes() (map[int16]bool, error) {
	recs, err := readCSV(mccCodesCSV, 3)
	if err != nil {
		return nil, fmt.Errorf("seeded mcc codes: %w", err)
	}
	out := make(map[int16]bool, len(recs))
	for _, rec := range recs {
		code, err := strconv.ParseInt(rec[0], 10, 16)
		if err != nil {
			return nil, fmt.Errorf("seeded mcc codes: bad code %q: %w", rec[0], err)
		}
		out[int16(code)] = true
	}
	return out, nil
}

// SeededMembershipKeys returns the (bank name, category title) pairs whose
// bank_category_mcc link sets seedMCC refreshes by diff — a foreign edit
// under one of these is reverted AND journalled as a false ±MCC event.
func SeededMembershipKeys() (map[[2]string]bool, error) {
	recs, err := readCSV(bankCategoryMCCCSV, 4)
	if err != nil {
		return nil, fmt.Errorf("seeded membership keys: %w", err)
	}
	out := make(map[[2]string]bool, len(recs))
	for _, rec := range recs {
		out[[2]string{rec[0], rec[1]}] = true
	}
	return out, nil
}
