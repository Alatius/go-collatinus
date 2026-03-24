package collatinus

import (
	"strings"
	"testing"
)

const dataDir = "data"

func TestNew(t *testing.T) {
	l, err := New(dataDir)
	if err != nil {
		t.Fatalf("New(%q): %v", dataDir, err)
	}
	if l == nil {
		t.Fatal("New returned nil Lemmatizer")
	}
	t.Logf("Loaded %d morpho languages (%d descriptions), %d models, %d lemmas, %d desinences, %d radicals, %d irregs",
		len(l.morphos), len(l.morphos["fr"])-1, len(l.models), len(l.lemmas),
		len(l.desinences), len(l.radicals), len(l.irregs))
}

func TestMorpho(t *testing.T) {
	l, _ := New(dataDir)
	got := l.Morpho(1)
	if got != "nominatif singulier" {
		t.Errorf("Morpho(1) = %q, want %q", got, "nominatif singulier")
	}
}

func TestMorphoLang(t *testing.T) {
	l, _ := New(dataDir)
	// French (default)
	if got := l.MorphoLang(1, "fr"); got != "nominatif singulier" {
		t.Errorf("MorphoLang(1, fr) = %q, want %q", got, "nominatif singulier")
	}
	// English
	if got := l.MorphoLang(1, "en"); got != "nominative singular" {
		t.Errorf("MorphoLang(1, en) = %q, want %q", got, "nominative singular")
	}
	// Unknown language falls back to French
	if got := l.MorphoLang(1, "xx"); got != "nominatif singulier" {
		t.Errorf("MorphoLang(1, xx) = %q, want French fallback %q", got, "nominatif singulier")
	}
	// Out of bounds
	if got := l.MorphoLang(0, "fr"); got != "" {
		t.Errorf("MorphoLang(0, fr) = %q, want empty", got)
	}
}

func TestMorphoLanguages(t *testing.T) {
	l, _ := New(dataDir)
	langs := l.MorphoLanguages()
	have := make(map[string]bool, len(langs))
	for _, lang := range langs {
		have[lang] = true
	}
	for _, want := range []string{"fr", "en", "es"} {
		if !have[want] {
			t.Errorf("MorphoLanguages() missing %q, got %v", want, langs)
		}
	}
}

func TestLemmaTranslation(t *testing.T) {
	l, _ := New(dataDir)
	lemma := l.Lemma("puella")
	if lemma == nil {
		t.Fatal("Lemma('puella') is nil")
	}
	tr := lemma.Translation("fr")
	if tr == "" {
		t.Error("puella.Translation('fr') is empty")
	} else {
		t.Logf("puella (fr) = %q", tr)
	}
}

func TestLemmatizeWordPuellae(t *testing.T) {
	l, _ := New(dataDir)
	result := l.LemmatizeWord("puellae", false)
	if len(result) == 0 {
		t.Fatal("LemmatizeWord('puellae') returned no results")
	}

	var foundLemma *Lemma
	for lemma := range result {
		if lemma.Key == "puella" || lemma.Grq == "puella" {
			foundLemma = lemma
			break
		}
	}
	if foundLemma == nil {
		t.Errorf("LemmatizeWord('puellae') did not find lemma 'puella'; got:")
		for lemma, analyses := range result {
			t.Logf("  %s: %v", lemma.Grq, analyses)
		}
		return
	}

	analyses := result[foundLemma]
	t.Logf("puellae analyses: %v", analyses)

	// Should include at least 2 analyses (gen sg + nom pl)
	if len(analyses) < 2 {
		t.Errorf("Expected >= 2 analyses for puellae, got %d", len(analyses))
	}
}

func TestLemmatizeWordAmat(t *testing.T) {
	l, _ := New(dataDir)
	result := l.LemmatizeWord("amat", false)
	if len(result) == 0 {
		t.Fatal("LemmatizeWord('amat') returned no results")
	}

	var foundLemma *Lemma
	for lemma := range result {
		if strings.HasPrefix(lemma.Grq, "amo") || lemma.Key == "amo" {
			foundLemma = lemma
			break
		}
	}
	if foundLemma == nil {
		t.Errorf("LemmatizeWord('amat') did not find lemma 'amo'; got:")
		for lemma, analyses := range result {
			t.Logf("  %s: %v", lemma.Grq, analyses)
		}
		return
	}
	t.Logf("amat analyses for 'amo': %v", result[foundLemma])
}

func TestInflectionTableLupus(t *testing.T) {
	l, _ := New(dataDir)
	lemma := l.Lemma("lupus")
	if lemma == nil {
		t.Fatal("Lemma('lupus') is nil")
	}
	table := l.InflectionTable(lemma)
	if table == nil {
		t.Fatal("InflectionTable returned nil")
	}
	t.Logf("lupus inflection table has %d cells", len(table.Cells))
	for mn, forms := range table.Cells {
		t.Logf("  morpho %d (%s): %v", mn, l.Morpho(mn), forms)
	}
	// Should have cells 1-12 for a 2nd declension noun
	for i := 1; i <= 12; i++ {
		if forms, ok := table.Cells[i]; !ok || len(forms) == 0 {
			t.Errorf("lupus inflection table missing or empty cell %d", i)
		}
	}
}

func TestLemmatizeWordNec(t *testing.T) {
	l, _ := New(dataDir)
	result := l.LemmatizeWord("nec", false)
	t.Logf("nec results: %d lemmas", len(result))
	for lemma, analyses := range result {
		t.Logf("  %s: %v", lemma.Grq, analyses)
	}
}

func TestEncliticStripping(t *testing.T) {
	l, _ := New(dataDir)
	result := l.LemmatizeWord("populusque", false)
	if len(result) == 0 {
		t.Fatal("LemmatizeWord('populusque') returned no results")
	}

	var foundLemma *Lemma
	for lemma := range result {
		// lemma.Gr is the canonical form without quantity marks or homonym digit
		if lemma.Gr == "populus" {
			foundLemma = lemma
			break
		}
	}
	if foundLemma == nil {
		t.Errorf("LemmatizeWord('populusque') did not find lemma 'populus'; got:")
		for lemma, analyses := range result {
			t.Logf("  %s: %v", lemma.Grq, analyses)
		}
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		fn   string
		in   string
		want string
	}{
		{"Deramise", "julius", "iulius"},
		{"Deramise", "Julius", "Iulius"},
		{"Deramise", "veni", "ueni"},
		{"Deramise", "Venus", "Uenus"}, // V is now replaced (new Ch::deramise)
		{"Atone", "ā", "a"},
		{"Atone", "ē", "e"},
		{"Atone", "ī", "i"},
		{"Atone", "ō", "o"},
		{"Atone", "ū", "u"},
		{"Atone", "ȳ", "y"},
		{"Atone", "Ā", "A"},
		{"Atone", "ā̆blŭo", "abluo"},
		{"NormalizeKey", "puella", "puella"},
		{"NormalizeKey", "pūella", "puella"},
	}
	for _, tt := range tests {
		var got string
		switch tt.fn {
		case "Deramise":
			got = Deramise(tt.in)
		case "Atone":
			got = Atone(tt.in)
		case "NormalizeKey":
			got = NormalizeKey(tt.in)
		}
		if got != tt.want {
			t.Errorf("%s(%q) = %q, want %q", tt.fn, tt.in, got, tt.want)
		}
	}
}

func TestListI(t *testing.T) {
	tests := []struct {
		in   string
		want []int
	}{
		{"1-6", []int{1, 2, 3, 4, 5, 6}},
		{"1,3,5", []int{1, 3, 5}},
		{"1-3,5,7-9", []int{1, 2, 3, 5, 7, 8, 9}},
		{"10", []int{10}},
	}
	for _, tt := range tests {
		got := ListI(tt.in)
		if len(got) != len(tt.want) {
			t.Errorf("ListI(%q) = %v, want %v", tt.in, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("ListI(%q)[%d] = %d, want %d", tt.in, i, got[i], tt.want[i])
			}
		}
	}
}
