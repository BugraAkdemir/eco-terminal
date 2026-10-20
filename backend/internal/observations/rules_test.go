package observations

import "testing"

func TestCanTransition(t *testing.T) {
	ok := []struct {
		from, to string
		kind     ActorKind
	}{
		{"draft", "submitted", ActorOwner}, {"submitted", "ai_processed", ActorAI}, {"submitted", "under_review", ActorReviewer},
		{"ai_processed", "verified", ActorReviewer}, {"under_review", "needs_information", ActorReviewer},
		{"needs_information", "under_review", ActorOwner}, {"verified", "resolved", ActorReviewer},
	}
	for _, c := range ok {
		if !CanTransition(c.from, c.to, c.kind) {
			t.Errorf("%s -> %s by %s should be allowed", c.from, c.to, c.kind)
		}
	}
	bad := []struct {
		from, to string
		kind     ActorKind
	}{
		{"submitted", "verified", ActorAI}, {"submitted", "rejected", ActorAI}, {"submitted", "verified", ActorOwner},
		{"draft", "verified", ActorReviewer}, {"rejected", "verified", ActorReviewer}, {"verified", "rejected", ActorReviewer},
		{"resolved", "verified", ActorReviewer}, {"under_review", "under_review", ActorReviewer}, {"needs_information", "verified", ActorReviewer},
		{"needs_information", "under_review", ActorReviewer},
	}
	for _, c := range bad {
		if CanTransition(c.from, c.to, c.kind) {
			t.Errorf("%s -> %s by %s must be refused", c.from, c.to, c.kind)
		}
	}
}

func TestPublicCoords(t *testing.T) {
	if lat, lon := PublicCoords(41.3887, 33.7827, false); lat != 41.3887 || lon != 33.7827 {
		t.Fatal("non-sensitive coordinates must stay exact")
	}
	if lat, lon := PublicCoords(41.3887, 33.7827, true); lat != 41.4 || lon != 33.8 {
		t.Fatalf("sensitive: %v %v", lat, lon)
	}
}
