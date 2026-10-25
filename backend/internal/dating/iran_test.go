package dating

import "testing"

func TestProvinceNames_ThirtyOne(t *testing.T) {
	names := ProvinceNames()
	if len(names) != 31 {
		t.Fatalf("province count = %d, want 31", len(names))
	}
	seen := map[string]bool{}
	for _, n := range names {
		if n == "" {
			t.Fatal("empty province name")
		}
		if seen[n] {
			t.Fatalf("duplicate province: %s", n)
		}
		seen[n] = true
	}
}

func TestProvinces_AllHaveCities(t *testing.T) {
	for name, cities := range PROVINCES {
		if len(cities) == 0 {
			t.Fatalf("province %s has no cities", name)
		}
		for _, c := range cities {
			if c == "" {
				t.Fatalf("province %s has an empty city", name)
			}
		}
	}
}

func TestCities_KnownProvince(t *testing.T) {
	cities := Cities("تهران")
	found := false
	for _, c := range cities {
		if c == "تهران" {
			found = true
		}
	}
	if !found {
		t.Fatalf("تهران must list itself as a city, got %v", cities)
	}
}

func TestCities_UnknownProvince(t *testing.T) {
	if Cities("ناموجود") != nil && len(Cities("ناموجود")) > 0 {
		t.Fatal("unknown province must return empty")
	}
}
