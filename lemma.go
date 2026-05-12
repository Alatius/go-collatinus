package collatinus

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Radical represents a stem used in inflection.
// Mirrors the Radical class in lemme.cpp.
type Radical struct {
	// Grq is the stem with vowel-quantity marks.
	Grq string
	// Gr is the stem without diacritics (Atone(Grq)).
	Gr string
	// Num is the radical number (1-based).
	Num int
	// Lemma is the lemma this radical belongs to.
	Lemma *Lemma
}

// Irreg represents an irregular inflected form.
// Mirrors the Irreg class in irregs.cpp.
type Irreg struct {
	// Grq is the form with vowel-quantity marks.
	Grq string
	// Gr is the form without diacritics (Atone(Grq)).
	Gr string
	// Exclusive indicates this form replaces (rather than supplements)
	// the regular inflection.
	Exclusive bool
	// Lemma is the lemma this irregular belongs to.
	Lemma *Lemma
	// Morphos lists the morpho indices this form covers.
	Morphos []int
}

// Lemma represents a dictionary headword with all its inflectional data.
// Mirrors the Lemme class in lemme.cpp.
type Lemma struct {
	// Key is the normalized lookup key (NormalizeKey of the entry key).
	Key string
	// Grq is the canonical form with vowel-quantity marks.
	Grq string
	// Gr is the canonical form without diacritics.
	Gr string
	// modelName is the name of the inflection model.
	modelName string
	// model is the resolved Model pointer.
	model *Model
	// IndMorph is the raw morphological-info string from lemmes.la.
	IndMorph string
	// POS is the part-of-speech.
	POS PartOfSpeech
	// HomonymNum is the homonym number (0 or 1 = primary, 2+ = secondary).
	HomonymNum int
	// renvoi is a cross-reference key (when IndMorph contains "cf. xxx").
	renvoi string

	// altGrqs holds additional canonical forms with quantity marks (comma-separated
	// alternatives after the first form in the lemmes.la Grq field).
	altGrqs []string

	// radicals maps radical-number → list of Radical pointers.
	radicals map[int][]*Radical
	// irregs is the list of irregular forms for this lemma.
	irregs []*Irreg
	// morphosIrregExcl lists morpho indices covered by exclusive irregulars.
	morphosIrregExcl []int
	// NbOcc is the occurrence count from lemmes.la (field 6).
	NbOcc int
	// Extended is true for lemmas loaded from lem_ext.la (vs. lemmes.la).
	Extended bool
	// translations maps language code → translation string.
	translations map[string]string
}

// cfRe matches "cf. <word>" at the end of indMorph.
var cfRe = regexp.MustCompile(`cf\.\s+(\w+)$`)

// newLemma parses a line from lemmes.la and creates a Lemma.
// Line format: key=grq|model|rad1|rad2|indMorph[|nbOcc]
// where key= part is optional (if absent, grq is used as key too).
func newLemma(line string) *Lemma {
	parts := strings.Split(line, "|")
	if len(parts) < 5 {
		return nil
	}

	l := &Lemma{
		radicals:     make(map[int][]*Radical),
		translations: make(map[string]string),
	}

	// Parse key=grq (or just grq)
	keyGrq := strings.SplitN(parts[0], "=", 2)
	rawKey := keyGrq[0]

	l.Key = NormalizeKey(rawKey)
	rawGrq := func() string {
		if len(keyGrq) > 1 {
			return keyGrq[1]
		}
		return rawKey
	}()
	// The Grq field may contain comma-separated alternative canonical forms
	// (e.g. "tēmpto,tēnto"). Use only the first as the primary Grq and
	// store the rest in altGrqs for radical building.
	grqForms := strings.SplitN(rawGrq, ",", -1)
	l.Grq, l.HomonymNum = oteNh(grqForms[0])
	l.Gr = Atone(l.Grq)
	for _, alt := range grqForms[1:] {
		alt = strings.TrimSpace(alt)
		if alt != "" {
			l.altGrqs = append(l.altGrqs, alt)
		}
	}

	l.modelName = parts[1]

	// Parse explicit radicals from fields 2 and 3 (1-indexed radical numbers)
	for i := 2; i < 4; i++ {
		if i >= len(parts) || parts[i] == "" {
			continue
		}
		radNum := i - 1 // field 2 → radical 1, field 3 → radical 2
		for _, radStr := range strings.Split(parts[i], ",") {
			if radStr == "" {
				continue
			}
			rad := &Radical{
				Grq:   Communes(radStr),
				Gr:    Atone(radStr),
				Num:   radNum,
				Lemma: l,
			}
			l.radicals[radNum] = append(l.radicals[radNum], rad)
		}
	}

	l.IndMorph = parts[4]
	l.POS = detectPOS(l.IndMorph)

	// Field 6: NbOcc (occurrence count)
	if len(parts) >= 6 && parts[5] != "" {
		l.NbOcc, _ = strconv.Atoi(parts[5])
	}

	// Cross-reference
	if m := cfRe.FindStringSubmatch(l.IndMorph); m != nil {
		l.renvoi = m[1]
	}

	return l
}

// oteNh strips the trailing homonym digit from g (if present) and returns
// (stripped string, homonymNum). Mirrors Lemme::oteNh.
func oteNh(g string) (string, int) {
	if g == "" {
		return g, 0
	}
	runes := []rune(g)
	last := runes[len(runes)-1]
	if unicode.IsDigit(last) {
		n, _ := strconv.Atoi(string(last))
		if n > 0 {
			return string(runes[:len(runes)-1]), n
		}
	}
	return g, 0
}

// detectPOS infers part of speech from the indMorph string.
// Mirrors the POS detection in Lemme::Lemme.
func detectPOS(indMorph string) PartOfSpeech {
	switch {
	case strings.Contains(indMorph, "adj."):
		return POSAdjective
	case strings.Contains(indMorph, "conj"):
		return POSConjunction
	case strings.Contains(indMorph, "excl"):
		return POSExclamation
	case strings.Contains(indMorph, "interj"):
		return POSInterjection
	case strings.Contains(indMorph, "num."):
		return POSNumeral
	case strings.Contains(indMorph, "pron."):
		return POSPronoun
	case strings.Contains(indMorph, "prép"):
		return POSPreposition
	case strings.Contains(indMorph, "adv"):
		return POSAdverb
	case strings.Contains(indMorph, " nom ") || strings.Contains(indMorph, "npr."):
		return POSNoun
	default:
		return POSUnknown
	}
}

// indMorphTranslations maps French terms in IndMorph to their English equivalents.
// Order matters: longer / more specific patterns must come first so that
// shorter patterns (e.g. " de ") do not consume text that a longer pattern
// would have matched.
var indMorphTranslations = []struct{ fr, en string }{
	// ---- Full unique phrases (most specific first) ----
	{"v. défectif utilisé seulement à la 3ème personne du singulier", "defective v. used only in 3rd person singular"},
	{"Pas de nominatif", "No nominative"},
	{"généralement au plur", "generally plural"},
	{"souvent au pluriel", "often plural"},
	{"surtout au pl.", "mostly pl."},
	{"toujours au pluriel", "always plural"},
	{"aussi en 2 mots", "also as 2 words"},
	{"comparatif neutre", "neuter comparative"},
	{"averbial", "adverbial"},
	{"suivi du nom.", "followed by nom."},

	{"inusité au nom.", "unused in nom."},
	{"inusité", "unused"},

	// ---- Miscellaneous French abbreviations ----
	{"qqf.", "sometimes"},
	{"qqf ", "sometimes "},
	{"mot grec", "Greek word"},

	// ---- "<description> de <word>" phrases (before generic " de ") ----
	{"part. passé de l'", "past part. of "},
	{"part. passé de ", "past part. of "},
	{"2ème p. s. de ", "2nd p. s. of "},
	{"part. fut. de ", "fut. part. of "},
	{"fut. ant. de ", "fut. ant. of "},
	{"acc. grec de ", "Greek acc. of "},
	{"acc. fem. sg. de ", "acc. fem. sg. of "},
	{"abl. n. de ", "abl. n. of "},
	{"abréviation de ", "abbreviation of "},
	{"supin en u de ", "u-supine of "},
	{"supin de ", "supine of "},
	{"comp. de ", "comp. of "},
	{"inf. de ", "inf. of "},
	{"acc. de ", "acc. of "},
	{"passif de ", "passive of "},

	// ---- Abbreviations and terms ----
	{"indécl.", "indecl."},
	{"défectif", "defective"},
	{"passif", "passive"},
	{"contr.", "contr."},
	{"prép.", "prep."},
	{"prép", "prep."},
	{"npr.", "prop.n."},
	{"+gén.", "+gen."},
	{"gén.", "gen."},
	{"gén,", "gen.,"},
	{"gén ", "gen. "},

	// ---- Common words (shortest / most generic last) ----
	{" nom ", " n. "},
	{" ou ", " or "},
	{" et ", " and "},
	{" de l'", " of "},
	{" de ", " of "},
}

// IndMorphLang returns the morphological-info string, translated for the
// given language code. For "fr" (or if no translation is available) the
// raw French string is returned as-is.
func (l *Lemma) IndMorphLang(lang string) string {
	if lang == "" || lang == "fr" {
		return l.IndMorph
	}
	s := l.IndMorph
	for _, t := range indMorphTranslations {
		s = strings.ReplaceAll(s, t.fr, t.en)
	}
	// Normalize bare "impers" (no dot) that appears at end of string.
	if strings.HasSuffix(s, "impers") {
		s += "."
	}
	return s
}

// Model returns the resolved Model for this lemma.
func (l *Lemma) Model() *Model {
	return l.model
}

// Translation returns the translation for language lang, falling back to "fr".
func (l *Lemma) Translation(lang string) string {
	if t, ok := l.translations[lang]; ok {
		return t
	}
	return l.translations["fr"]
}

// AddTranslation adds a translation for the given language code.
func (l *Lemma) AddTranslation(lang, text string) {
	l.translations[lang] = text
}

// addIrreg attaches an irregular form to this lemma.
func (l *Lemma) addIrreg(irr *Irreg) {
	l.irregs = append(l.irregs, irr)
	if irr.Exclusive {
		l.morphosIrregExcl = append(l.morphosIrregExcl, irr.Morphos...)
	}
}

// isExclusiveIrreg returns true if morpho index nm is covered by an exclusive irregular.
func (l *Lemma) isExclusiveIrreg(nm int) bool {
	for _, v := range l.morphosIrregExcl {
		if v == nm {
			return true
		}
	}
	return false
}

// irregAt returns the irregular form for morpho index i, and whether it's exclusive.
// Mirrors Lemme::irreg.
func (l *Lemma) irregAt(i int) (string, bool) {
	for _, ir := range l.irregs {
		for _, m := range ir.Morphos {
			if m == i {
				return ir.Grq, ir.Exclusive
			}
		}
	}
	return "", false
}

// RadicalsAt returns all radicals for radical number r.
func (l *Lemma) RadicalsAt(r int) []*Radical {
	return l.radicals[r]
}
