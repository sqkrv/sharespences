package auth

import (
	"testing"
	"time"
)

// An unknown email must cost the same as a wrong password, or the response
// time answers «does this address have an account here?» on its own.
func TestLoginTimingIsFlatForUnknownEmails(t *testing.T) {
	real, err := HashPassword("correcthorsebatterystaple")
	if err != nil {
		t.Fatal(err)
	}

	measure := func(fn func()) time.Duration {
		start := time.Now()
		fn()
		return time.Since(start)
	}
	wrongPassword := measure(func() { VerifyPassword("nope", real) })
	unknownEmail := measure(func() { VerifyPassword("nope", dummyHash) })

	// Same argon2 parameters on both sides, so the two must be the same order
	// of magnitude — the failure this guards against is one of them being ~0.
	ratio := float64(wrongPassword) / float64(unknownEmail)
	if ratio < 0.25 || ratio > 4 {
		t.Fatalf("wrong password %v vs unknown email %v: ratio %.2f, want within 4x", wrongPassword, unknownEmail, ratio)
	}
}
