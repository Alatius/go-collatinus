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

// TestAssimilationFallback verifies that when a form has to be rewritten
// (e.g. "attingo" → "adtingo") to be lemmatized, the returned marked form
// is re-assimilated to match the caller's input. Mirrors the C++ lemmatiseM
// case 2 post-processing with assimq/desassimq.
func TestAssimilationFallback(t *testing.T) {
	l, _ := New(dataDir)

	findAdtingo := func(result map[*Lemma][]Analysis) *Lemma {
		for lemma := range result {
			if lemma.Key == "adtingo" {
				return lemma
			}
		}
		return nil
	}

	// "attingo" has no direct radical match (only "adting" and "attig" are
	// registered); it must go through desassim → "adtingo" and then the
	// resulting "ādtīngō̆" must be re-assimilated to "āttīngō̆".
	result := l.LemmatizeWord("attingo", false)
	lemma := findAdtingo(result)
	if lemma == nil {
		t.Fatalf("LemmatizeWord('attingo') did not find lemma adtingo; got %v", result)
	}
	var gotMarks []string
	for _, a := range result[lemma] {
		gotMarks = append(gotMarks, a.FormWithMarks)
	}
	hasAtt := false
	for _, g := range gotMarks {
		if strings.HasPrefix(g, "ātt") {
			hasAtt = true
			break
		}
	}
	if !hasAtt {
		t.Errorf("LemmatizeWord('attingo') expected an 'ātt...'-form for adtingo, got %v", gotMarks)
	}

	// "adtingo" is the un-assimilated input; the returned marked form must
	// start with "ādt" (not be accidentally rewritten by desassim post-processing).
	result = l.LemmatizeWord("adtingo", false)
	lemma = findAdtingo(result)
	if lemma == nil {
		t.Fatalf("LemmatizeWord('adtingo') did not find lemma adtingo; got %v", result)
	}
	gotMarks = gotMarks[:0]
	for _, a := range result[lemma] {
		gotMarks = append(gotMarks, a.FormWithMarks)
	}
	hasAdt := false
	for _, g := range gotMarks {
		if strings.HasPrefix(g, "ādt") {
			hasAdt = true
			break
		}
	}
	if !hasAdt {
		t.Errorf("LemmatizeWord('adtingo') expected an 'ādt...'-form for adtingo, got %v", gotMarks)
	}

	// "adcedo" exercises the assim branch: lexicon has only the assimilated
	// lemma "accedo", so assim("adcedo")="accedo" finds it, and the returned
	// marked form "āccēdō̆" must be rewritten via desassimq to "ādcēdō̆" to
	// reflect the caller's un-assimilated input.
	result = l.LemmatizeWord("adcedo", false)
	var accedo *Lemma
	for lem := range result {
		if lem.Key == "accedo" {
			accedo = lem
			break
		}
	}
	if accedo == nil {
		t.Fatalf("LemmatizeWord('adcedo') did not find lemma accedo; got %v", result)
	}
	gotMarks = gotMarks[:0]
	for _, a := range result[accedo] {
		gotMarks = append(gotMarks, a.FormWithMarks)
	}
	hasAdc := false
	for _, g := range gotMarks {
		if strings.HasPrefix(g, "ādc") {
			hasAdc = true
			break
		}
	}
	if !hasAdc {
		t.Errorf("LemmatizeWord('adcedo') expected an 'ādc...'-form for accedo, got %v", gotMarks)
	}
}

// TestDoubleIJAdjacent covers the ii-ambiguity handler when the matched
// Grq spells the user's 'i' as 'j' (e.g. "conicio" → alt "cōnjĭcĭo"):
// the char at the removal position is a real ĭ that belongs next to
// the j, and removing it would strand the j against the following 'c'
// ("cōnjcĭō̆"). The handler must keep the full marked form instead.
//
// The two positive cases pin the canonical bug. The bulk iteration
// walks every iacio-compound with a j/i alt form in the lexicon and
// asserts the invariant "no 'j' stranded before a consonant" on all
// returned forms — guarding against the same structural bug on any
// other entry in this family, even when only one of the j/i variants
// is reached by the matching.
func TestDoubleIJAdjacent(t *testing.T) {
	lem, _ := New(dataDir)

	type wantCase struct{ form, lemmaKey, want string }
	wants := []wantCase{
		{"conicio", "conicio", "cōnjĭcĭō̆"},
		{"eicio", "eiicio", "ējĭcĭō̆"},
	}
	for _, c := range wants {
		result := lem.LemmatizeWord(c.form, false)
		var target *Lemma
		for k := range result {
			if k.Key == c.lemmaKey {
				target = k
				break
			}
		}
		if target == nil {
			t.Errorf("LemmatizeWord(%q) did not find lemma %q", c.form, c.lemmaKey)
			continue
		}
		found := false
		var got []string
		for _, a := range result[target] {
			got = append(got, a.FormWithMarks)
			if a.FormWithMarks == c.want {
				found = true
			}
		}
		if !found {
			t.Errorf("LemmatizeWord(%q) lemma %q: want form %q, got %v",
				c.form, c.lemmaKey, c.want, got)
		}
	}

	// All j/i iacio-compounds in the lexicon. Each is probed from both
	// directions (i-form and j-form); every returned form_with_marks
	// must not strand 'j' before a consonant.
	forms := []string{
		"abicio", "abjicio", "adicio", "adjicio",
		"circumicio", "circumjicio", "conicio", "conjicio",
		"deicio", "dejicio", "disicio", "disjicio",
		"eicio", "ejicio", "inicio", "injicio",
		"intericio", "interjicio", "obicio", "objicio",
		"proicio", "projicio", "reicio", "rejicio",
		"subicio", "subjicio", "superinicio", "superinjicio",
		"traicio", "trajicio",
	}
	for _, form := range forms {
		for _, as := range lem.LemmatizeWord(form, false) {
			for _, a := range as {
				if strings.Contains(a.FormWithMarks, "jc") {
					t.Errorf("LemmatizeWord(%q): form %q has j stranded before c",
						form, a.FormWithMarks)
				}
			}
		}
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

func TestIndMorphLang(t *testing.T) {
	tests := []struct {
		indMorph string
		lang     string
		want     string
	}{
		// French passthrough.
		{"is, ire, ii ou ivi, itum", "fr", "is, ire, ii ou ivi, itum"},
		{"is, ire, ii ou ivi, itum", "", "is, ire, ii ou ivi, itum"},
		// No French terms — unchanged.
		{"a, um", "en", "a, um"},
		// Simple word replacements.
		{"is, ire, ii ou ivi, itum", "en", "is, ire, ii or ivi, itum"},
		{"m. ou f.", "en", "m. or f."},
		{"adj. et subst.", "en", "adj. and subst."},
		// Abbreviations.
		{"indécl.", "en", "indecl."},
		{"prép.", "en", "prep."},
		{"prép. + gén.", "en", "prep. + gen."},
		{"prép. + gén. ou abl.", "en", "prep. + gen. or abl."},
		{"npr. m.", "en", "prop.n. m."},
		{"défectif", "en", "defective"},
		{"+gén.", "en", "+gen."},
		{"v. impers.", "en", "v. impers."},
		{"v. impers", "en", "v. impers."},
		// Passif de.
		{"fis, fĭĕri, factus sum (passif de calefacio)", "en", "fis, fĭĕri, factus sum (passive of calefacio)"},
		// Other "de" phrases.
		{"supin de facio", "en", "supine of facio"},
		{"supin en u de dicere", "en", "u-supine of dicere"},
		{"inf. de possum", "en", "inf. of possum"},
		{"comp. de sancte", "en", "comp. of sancte"},
		{"abréviation de salutem [dat]", "en", "abbreviation of salutem [dat]"},
		{"acc. grec de Styx, Stygos", "en", "Greek acc. of Styx, Stygos"},
		{"acc. fem. sg. de ecqui, equae, ecquod", "en", "acc. fem. sg. of ecqui, equae, ecquod"},
		{"abl. n. de propatulus", "en", "abl. n. of propatulus"},
		{"a, um, part. fut. de sum", "en", "a, um, fut. part. of sum"},
		{"fut. ant. de facio", "en", "fut. ant. of facio"},
		{"acc. de ususfructus", "en", "acc. of ususfructus"},
		{"2ème p. s. de inquam", "en", "2nd p. s. of inquam"},
		// Full long phrase.
		{"v. défectif utilisé seulement à la 3ème personne du singulier", "en",
			"defective v. used only in 3rd person singular"},
		// Parenthetical French.
		{"gén, acc. vicem. Pas de nominatif", "en", "gen., acc. vicem. No nominative"},
		{"i, n. (généralement au plur)", "en", "i, n. (generally plural)"},
		{"ae, f. (souvent au pluriel)", "en", "ae, f. (often plural)"},
		{"is, f. (surtout au pl.)", "en", "is, f. (mostly pl.)"},
		{"i (toujours au pluriel)", "en", "i (always plural)"},
		{"jurisjurandi (aussi en 2 mots)", "en", "jurisjurandi (also as 2 words)"},
		// Combined: comparatif neutre ou averbial de elegans.
		{"comparatif neutre ou averbial de elegans, antis", "en",
			"neuter comparative or adverbial of elegans, antis"},
		// Suivi du nom.
		{"suivi du nom. ou de l'acc.", "en", "followed by nom. or of acc."},
		// Extended lexicon terms.
		{"antis (part. passé de l'inusité amaro)", "en", "antis (past part. of unused amaro)"},
		{"ōnis, f. (inusité au nom.)", "en", "ōnis, f. (unused in nom.)"},
	}
	for _, tt := range tests {
		l := &Lemma{IndMorph: tt.indMorph}
		got := l.IndMorphLang(tt.lang)
		if got != tt.want {
			t.Errorf("IndMorphLang(%q, %q) = %q, want %q", tt.indMorph, tt.lang, got, tt.want)
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

// TestAssimLongestFirst verifies that assim() picks the longest matching
// prefix. "adsto" has two matching keys in the assim table ("ads" → "ass"
// and "adst" → "ast"); the longer "adst" must win, transforming "adsto"
// → "asto" so that the assim fallback finds the astutus adjective.
// Previously Go's randomized map iteration sometimes picked "ads" first,
// producing the nonsensical "assto" which matched nothing.
func TestAssimLongestFirst(t *testing.T) {
	l, _ := New(dataDir)
	if got, want := l.assim("adsto"), "asto"; got != want {
		t.Errorf("assim(\"adsto\") = %q, want %q", got, want)
	}
	result := l.LemmatizeWord("adsto", false)
	var haveAdsto, haveAstutus bool
	for lemma := range result {
		switch lemma.Key {
		case "adsto":
			haveAdsto = true
		case "astutus":
			haveAstutus = true
		}
	}
	if !haveAdsto {
		t.Errorf("missing direct lemma 'adsto'")
	}
	if !haveAstutus {
		t.Errorf("missing assim-fallback lemma 'astutus' — longest-first must pick 'adst' over 'ads'")
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
