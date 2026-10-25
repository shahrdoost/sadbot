package dating

import "testing"

func TestSimilarityScore_CityBeatsProvince(t *testing.T) {
	a := Searcher{Gender: "male", City: "تهران", Province: "تهران", Age: 25, Filter: "any"}
	b := Searcher{Gender: "female", City: "تهران", Province: "تهران", Age: 27, Filter: "any"}
	c := Searcher{Gender: "female", City: "کرج", Province: "البرز", Age: 27, Filter: "any"}

	if similarityScore(a, b) <= similarityScore(a, c) {
		t.Fatalf("same city must score higher: same=%d other=%d", similarityScore(a, b), similarityScore(a, c))
	}
}

func TestSimilarityScore_Weights(t *testing.T) {
	a := Searcher{Gender: "male", City: "تهران", Province: "تهران", Age: 25, Interests: []string{"کتاب", "موسیقی"}, Filter: "any"}
	b := Searcher{Gender: "female", City: "تهران", Province: "تهران", Age: 26, Interests: []string{"کتاب", "ورزش"}, Filter: "any"}
	// gender 10 + city 8 + age≤3: 5 + shared interest 3 = 26
	if got := similarityScore(a, b); got != 26 {
		t.Fatalf("expected 26, got %d", got)
	}
}

func TestGenderCompatible_Filters(t *testing.T) {
	male := Searcher{Gender: "male", Filter: "any"}
	femaleAny := Searcher{Gender: "female", Filter: "any"}
	femaleWantMale := Searcher{Gender: "female", Filter: "male"}
	maleWantFemale := Searcher{Gender: "male", Filter: "female"}

	if !genderCompatible(male, femaleAny) {
		t.Fatal("any/any must be compatible")
	}
	if !genderCompatible(maleWantFemale, femaleWantMale) {
		t.Fatal("cross filters must be compatible")
	}
	if !genderCompatible(male, femaleWantMale) {
		t.Fatal("unfiltered male is compatible with female seeking male")
	}
	boySeekingGirl := Searcher{Gender: "male", Filter: "female"}
	girlSeekingGirl := Searcher{Gender: "female", Filter: "female"}
	if genderCompatible(boySeekingGirl, girlSeekingGirl) {
		t.Fatal("boy seeking girl must not match girl seeking girl")
	}
}

func TestGenderCompatible_BothSides(t *testing.T) {
	girlSeekingGirl := Searcher{Gender: "female", Filter: "female"}
	boy := Searcher{Gender: "male", Filter: "any"}
	if genderCompatible(girlSeekingGirl, boy) {
		t.Fatal("girl seeking girl must not match a boy")
	}
}

func TestSimilarityScore_AgeBuckets(t *testing.T) {
	base := Searcher{Gender: "male", City: "تهران", Province: "تهران", Age: 25, Filter: "any"}
	near := Searcher{Gender: "female", City: "تهران", Province: "تهران", Age: 27, Filter: "any"} // ≤3
	mid := Searcher{Gender: "female", City: "تهران", Province: "تهران", Age: 30, Filter: "any"}  // ≤5
	far := Searcher{Gender: "female", City: "تهران", Province: "تهران", Age: 40, Filter: "any"}  // none

	sNear := similarityScore(base, near)
	sMid := similarityScore(base, mid)
	sFar := similarityScore(base, far)
	if !(sNear > sMid && sMid > sFar) {
		t.Fatalf("age buckets must strictly decrease: near=%d mid=%d far=%d", sNear, sMid, sFar)
	}
	if sNear-sMid != 3 { // 5 vs 2
		t.Fatalf("bucket gap = %d, want 3", sNear-sMid)
	}
}

func TestSimilarityScore_ProvinceOnly(t *testing.T) {
	a := Searcher{Gender: "male", City: "تبریز", Province: "آذربایجان شرقی", Age: 25, Filter: "any"}
	b := Searcher{Gender: "female", City: "مراغه", Province: "آذربایجان شرقی", Age: 25, Filter: "any"}
	// gender 10 + province 4 + age 5 = 19
	if got := similarityScore(a, b); got != 19 {
		t.Fatalf("expected 19, got %d", got)
	}
}
