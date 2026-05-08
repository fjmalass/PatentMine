package domain

import (
	"testing"
	"time"
)

func TestPatentIsExpired(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	if !((Patent{ExpirationDate: "2020-01-01"}).IsExpired(now)) {
		t.Fatal("expected past expiration date to be expired")
	}
	if (Patent{ExpirationDate: "2043-03-21"}).IsExpired(now) {
		t.Fatal("expected future expiration date to be active")
	}
	if (Patent{}).IsExpired(now) {
		t.Fatal("expected missing expiration date to be active/unknown")
	}
}
