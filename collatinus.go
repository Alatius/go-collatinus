// Package collatinus provides Latin morphological analysis and lemmatization,
// parsing the same data files as the Collatinus-11 C++/Qt application
// without any Qt dependency.
package collatinus

// Lemmatizer holds all loaded data and provides the public API.
type Lemmatizer struct {
	// morphos stores morphological descriptions per language, each indexed 1-based.
	// Index 0 is unused; morphos["fr"][1] = "nominatif singulier", etc.
	morphos map[string][]string

	// models maps model name → *Model.
	models map[string]*Model

	// lemmas maps NormalizeKey(entry) → *Lemma.
	lemmas map[string]*Lemma

	// desinences maps Deramise(Atone(ending)) → []*Desinence.
	desinences map[string][]*Desinence

	// radicals maps Deramise(Atone(stem)) → []*Radical.
	radicals map[string][]*Radical

	// irregs maps Deramise(Atone(form)) → []*Irreg.
	irregs map[string][]*Irreg

	// variables stores $name=value substitutions used in modeles.la.
	variables map[string]string

	// languages maps language code (e.g. "fr") → language name.
	languages map[string]string

	// assims maps non-assimilated prefix → assimilated prefix.
	assims map[string]string
	// assimsByKey is the same mapping as a list, sorted longest-first by
	// key. Iterated by assim() so that the longest matching prefix wins
	// deterministically (Go's randomized map iteration could otherwise
	// pick a shorter prefix, e.g. "ads" before "adst").
	assimsByKey []assimEntry
	// assimsByVal is the same list, sorted longest-first by value.
	// Iterated by desassim() for the same reason.
	assimsByVal []assimEntry

	// contractions maps contracted ending → expanded ending.
	contractions map[string]string
}

// assimEntry is a single (unassimilated, assimilated) pair from the
// assimilation table, used in the sorted iteration slices.
type assimEntry struct {
	key string // e.g. "adt"
	val string // e.g. "att"
}

// New loads all Collatinus data from dataDir (the path to bin/data/)
// and returns a ready-to-use Lemmatizer.
func New(dataDir string) (*Lemmatizer, error) {
	l := &Lemmatizer{
		morphos:      make(map[string][]string),
		models:       make(map[string]*Model),
		lemmas:       make(map[string]*Lemma),
		desinences:   make(map[string][]*Desinence),
		radicals:     make(map[string][]*Radical),
		irregs:       make(map[string][]*Irreg),
		variables:    make(map[string]string),
		languages:    make(map[string]string),
		assims:       make(map[string]string),
		contractions: make(map[string]string),
	}

	if err := l.loadAssims(dataDir); err != nil {
		return nil, err
	}
	if err := l.loadContractions(dataDir); err != nil {
		return nil, err
	}

	if err := l.loadMorphos(dataDir); err != nil {
		return nil, err
	}
	if err := l.loadModels(dataDir); err != nil {
		return nil, err
	}
	if err := l.loadLexicon(dataDir); err != nil {
		return nil, err
	}
	if err := l.loadExtendedLexicon(dataDir); err != nil {
		return nil, err
	}
	if err := l.loadTranslations(dataDir); err != nil {
		return nil, err
	}
	if err := l.loadIrregs(dataDir); err != nil {
		return nil, err
	}
	// parpos.txt is loaded separately (not needed for core lemmatization)
	return l, nil
}

// Morpho returns the French morphological description string for 1-based index m.
// Mirrors Lemmat::morpho.
func (l *Lemmatizer) Morpho(m int) string {
	return l.MorphoLang(m, "fr")
}

// MorphoLang returns the morphological description for 1-based index m
// in the given language. Falls back to "fr" if lang is not available.
func (l *Lemmatizer) MorphoLang(m int, lang string) string {
	if s, ok := l.morphos[lang]; ok && m >= 1 && m < len(s) {
		return s[m]
	}
	if s := l.morphos["fr"]; m >= 1 && m < len(s) {
		return s[m]
	}
	return ""
}

// MorphoLanguages returns the language codes for which morphological
// descriptions are available (e.g. "fr", "en", "es", "k9").
func (l *Lemmatizer) MorphoLanguages() []string {
	out := make([]string, 0, len(l.morphos))
	for k := range l.morphos {
		out = append(out, k)
	}
	return out
}

// Lemma looks up a lemma by its normalized key.
func (l *Lemmatizer) Lemma(key string) *Lemma {
	return l.lemmas[NormalizeKey(key)]
}

// LemmaByKey looks up a lemma by its already-normalized key.
func (l *Lemmatizer) LemmaByKey(key string) *Lemma {
	return l.lemmas[key]
}

// Languages returns a map of language-code → language-name for all
// loaded translation files.
func (l *Lemmatizer) Languages() map[string]string {
	out := make(map[string]string, len(l.languages))
	for k, v := range l.languages {
		out[k] = v
	}
	return out
}

// LemmatizeWord lemmatizes a single Latin word form.
// If sentenceStart is true the word may be capitalized because it
// is the first word of a sentence (not necessarily a proper noun).
// Mirrors Lemmat::lemmatiseM.
func (l *Lemmatizer) LemmatizeWord(form string, sentenceStart bool) map[*Lemma][]Analysis {
	return l.lemmatizeM(form, sentenceStart)
}

// LemmatizeText splits text into tokens and lemmatizes each word.
func (l *Lemmatizer) LemmatizeText(text string) []LemmatizationResult {
	return l.lemmatizeText(text)
}

// InflectionTable computes the full inflection table for a lemma.
func (l *Lemmatizer) InflectionTable(lemma *Lemma) *InflectionTable {
	return l.inflectionTable(lemma)
}

// addDesinence inserts a desinence into the global desinences map.
// Mirrors Lemmat::ajDesinence.
func (l *Lemmatizer) addDesinence(d *Desinence) {
	key := Deramise(d.Gr)
	l.desinences[key] = append(l.desinences[key], d)
}

// addRadical inserts a radical into the global radicals map.
// Mirrors the insert call in Lemmat::ajRadicaux.
func (l *Lemmatizer) addRadical(r *Radical) {
	key := Deramise(r.Gr)
	l.radicals[key] = append(l.radicals[key], r)
}

// FilterExtended removes extension-only lemmas from results when
// main-lexicon lemmas are also present. If all results come from the
// extension, they are kept. Mirrors the C++ _extension filter in
// LemCore::lemmatise.
func FilterExtended(results map[*Lemma][]Analysis) map[*Lemma][]Analysis {
	if len(results) == 0 {
		return results
	}
	filtered := make(map[*Lemma][]Analysis)
	for l, a := range results {
		if !l.Extended {
			filtered[l] = a
		}
	}
	if len(filtered) == 0 {
		return results // all results are from extension; keep them
	}
	return filtered
}
