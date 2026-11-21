package clock

import (
	"testing"
	"time"
)

func TestFixed(t *testing.T) {
	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	var c Clock = Fixed(at)
	if !c.Now().Equal(at) {
		t.Fatalf("Now() = %v, want %v", c.Now(), at)
	}
}

func TestRealMoves(t *testing.T) {
	var c Clock = Real{}
	if c.Now().IsZero() {
		t.Fatal("Real.Now() is zero")
	}
}
