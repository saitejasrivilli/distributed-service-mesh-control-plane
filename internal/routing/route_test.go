package routing

import "testing"

func validSpec() Spec {
	return Spec{
		Service: "backend-a",
		Splits: []VersionWeight{
			{Version: "v1", Weight: 90},
			{Version: "v2", Weight: 10},
		},
	}
}

func TestValidateAcceptsWeightsSummingTo100(t *testing.T) {
	if err := validSpec().Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateRejectsWeightsNotSummingTo100(t *testing.T) {
	s := validSpec()
	s.Splits[1].Weight = 5
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for weights summing to 95")
	}
}

func TestValidateRejectsEmptySplits(t *testing.T) {
	s := Spec{Service: "backend-a"}
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for empty splits")
	}
}

func TestValidateRejectsMissingService(t *testing.T) {
	s := validSpec()
	s.Service = ""
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for missing service")
	}
}

func TestValidateRejectsDuplicateVersion(t *testing.T) {
	s := validSpec()
	s.Splits = append(s.Splits, VersionWeight{Version: "v1", Weight: 0})
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for duplicate version")
	}
}

func TestValidateRejectsRetryOnWithoutNumRetries(t *testing.T) {
	s := validSpec()
	s.RetryOn = "5xx"
	if err := s.Validate(); err == nil {
		t.Fatal("expected error for retry_on set without num_retries")
	}
}

func TestStoreSetRejectsInvalidSpec(t *testing.T) {
	store := NewStore()
	err := store.Set(Spec{Service: "backend-a"})
	if err == nil {
		t.Fatal("expected error for invalid spec")
	}
	if _, ok := store.Get("backend-a"); ok {
		t.Fatal("invalid spec must not be stored")
	}
}

func TestStoreSetAndGet(t *testing.T) {
	store := NewStore()
	if err := store.Set(validSpec()); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, ok := store.Get("backend-a")
	if !ok {
		t.Fatal("expected spec to be present")
	}
	if len(got.Splits) != 2 {
		t.Fatalf("splits = %d, want 2", len(got.Splits))
	}
}

func TestStoreSetOverwritesPrevious(t *testing.T) {
	store := NewStore()
	_ = store.Set(validSpec())
	updated := Spec{Service: "backend-a", Splits: []VersionWeight{{Version: "v2", Weight: 100}}}
	if err := store.Set(updated); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, _ := store.Get("backend-a")
	if len(got.Splits) != 1 || got.Splits[0].Version != "v2" {
		t.Fatalf("got %+v, want single v2 split", got)
	}
}

func TestStoreDelete(t *testing.T) {
	store := NewStore()
	_ = store.Set(validSpec())
	store.Delete("backend-a")
	if _, ok := store.Get("backend-a"); ok {
		t.Fatal("expected spec to be removed")
	}
}

func TestStoreGetUnknownServiceReturnsFalse(t *testing.T) {
	store := NewStore()
	if _, ok := store.Get("nope"); ok {
		t.Fatal("expected ok=false for unknown service")
	}
}
