package seed

// Coverage set for the admin sidecar (ADR-0008): «if seed writes it, the
// panel only reads it». Parsed from the same embedded CSV seedMCC writes
// from, so the guard can never drift from the behavior. Membership has no
// set any more — it left the seed with the ADR-0004 import (see mcc.go).

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

