package collatinus

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// loadMorphos reads all data/morphos.* files into l.morphos (1-based, per language).
// Format: "n:description" (1-indexed), stops at "! --- " separator.
// Mirrors LemCore::lisMorphos.
func (l *Lemmatizer) loadMorphos(dataDir string) error {
	matches, err := filepath.Glob(filepath.Join(dataDir, "morphos.*"))
	if err != nil {
		return err
	}

	for _, path := range matches {
		ext := filepath.Ext(path)
		if ext == "" || ext == ".la" {
			continue
		}
		lang := ext[1:] // e.g. "fr", "en", "es", "k9"

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open %s: %w", filepath.Base(path), err)
		}

		descs := []string{""} // index 0 unused; 1-based
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "! --- ") {
				break
			}
			if strings.HasPrefix(line, "!") {
				continue
			}
			idx := strings.Index(line, ":")
			if idx < 0 {
				continue
			}
			descs = append(descs, line[idx+1:])
		}
		scanErr := sc.Err()
		f.Close()
		if scanErr != nil {
			return scanErr
		}
		l.morphos[lang] = descs
	}

	if _, ok := l.morphos["fr"]; !ok {
		return fmt.Errorf("morphos.fr not found in %s", dataDir)
	}
	return nil
}

// loadModels reads bin/data/modeles.la and populates l.models.
// Also registers all desinences into l.desinences.
// Mirrors Lemmat::lisModeles.
func (l *Lemmatizer) loadModels(dataDir string) error {
	f, err := os.Open(filepath.Join(dataDir, "modeles.la"))
	if err != nil {
		return fmt.Errorf("open modeles.la: %w", err)
	}
	defer f.Close()

	var block []string
	sc := bufio.NewScanner(f)
	atEOF := false

	flushBlock := func() {
		if len(block) == 0 {
			return
		}
		m := l.parseModel(block)
		if m != nil {
			l.models[m.Name] = m
		}
		block = block[:0]
	}

	for !atEOF {
		var line string
		if sc.Scan() {
			line = strings.TrimSpace(sc.Text())
		} else {
			atEOF = true
			// flush remaining block at EOF
		}

		if line == "" && !atEOF {
			continue
		}
		if strings.HasPrefix(line, "!") {
			continue
		}

		// Variables: $name=value
		if strings.HasPrefix(line, "$") {
			idx := strings.Index(line, "=")
			if idx > 0 {
				l.variables[line[:idx]] = line[idx+1:]
			}
			continue
		}

		// When we see a new "modele:" line (or EOF), flush the accumulated block
		parts := strings.SplitN(line, ":", 2)
		if (parts[0] == "modele" || atEOF) && len(block) > 0 {
			flushBlock()
		}

		if !atEOF {
			block = append(block, line)
		}
	}
	return sc.Err()
}

// parseModel builds a Model from a block of lines from modeles.la.
// Mirrors Modele::Modele constructor.
func (l *Lemmatizer) parseModel(lines []string) *Model {
	m := newModel("")

	// multimap for suffixes: suffix → []morphoNums
	type suffEntry struct {
		suf    string
		morpho int
	}
	var sufEntries []suffEntry

	for _, line := range lines {
		// variable substitution
		line = l.substituteVars(line)

		eclats := strings.Split(strings.TrimSpace(line), ":")

		switch eclats[0] {
		case "modele":
			if len(eclats) > 1 {
				m.Name = eclats[1]
			}
		case "pere":
			if len(eclats) > 1 {
				m.parent = l.models[eclats[1]]
			}
		case "des", "des+":
			if len(eclats) < 4 {
				continue
			}
			morphoNums := ListI(eclats[1])
			radNum, _ := strconv.Atoi(eclats[2])
			desStrs := strings.Split(eclats[3], ";")

			for i, mn := range morphoNums {
				var desStr string
				if i < len(desStrs) {
					desStr = desStrs[i]
				} else if len(desStrs) > 0 {
					desStr = desStrs[len(desStrs)-1]
				}
				// Each desStr may be comma-separated (multiple desinences for same morpho)
				for _, g := range strings.Split(desStr, ",") {
					grq := g
					if grq == "-" {
						grq = ""
					}
					d := &Desinence{
						Grq:       grq,
						Gr:        Atone(grq),
						MorphoNum: mn,
						RadNum:    radNum,
						Model:     m,
					}
					m.Desinences[mn] = append(m.Desinences[mn], d)
					l.addDesinence(d)
				}
			}

			// des+: also inherit parent desinences for the listed morphos
			if eclats[0] == "des+" && m.parent != nil {
				for _, mn := range morphoNums {
					for _, dp := range m.parent.Desinences[mn] {
						dc := cloneDesinence(dp, m)
						m.Desinences[mn] = append(m.Desinences[mn], dc)
						l.addDesinence(dc)
					}
				}
			}

		case "R":
			if len(eclats) < 3 {
				continue
			}
			rn, _ := strconv.Atoi(eclats[1])
			m.RadicalRules[rn] = eclats[2]

		case "abs":
			if len(eclats) > 1 {
				m.Absents = ListI(eclats[1])
			}

		case "abs+":
			if len(eclats) > 1 {
				m.Absents = append(m.Absents, ListI(eclats[1])...)
			}

		case "pos":
			if len(eclats) > 1 && len(eclats[1]) > 0 {
				m.pos = rune(eclats[1][0])
			}

		case "suf":
			// suf:<interval>:<suffix>
			if len(eclats) < 3 {
				continue
			}
			suf := eclats[2]
			for _, mn := range ListI(eclats[1]) {
				sufEntries = append(sufEntries, suffEntry{suf, mn})
			}

		case "sufd":
			// sufd:<suffix> — take all parent desinences and suffix them
			if m.parent == nil || len(eclats) < 2 {
				continue
			}
			suf := eclats[1]
			for _, dp := range m.parent.AllDesinences() {
				if m.isAbsent(dp.MorphoNum) {
					continue
				}
				grq := dp.Grq + suf
				d := &Desinence{
					Grq:       grq,
					Gr:        Atone(grq),
					MorphoNum: dp.MorphoNum,
					RadNum:    dp.RadNum,
					Model:     m,
				}
				m.Desinences[dp.MorphoNum] = append(m.Desinences[dp.MorphoNum], d)
				l.addDesinence(d)
			}
		}
	}

	// Inherit pos from parent if not set in child
	if m.pos == 0 && m.parent != nil {
		m.pos = m.parent.pos
	}

	// Inherit from parent (for morpho indices not already in child and not absent)
	if m.parent != nil {
		for mn, parentDes := range m.parent.Desinences {
			if m.hasDesinence(mn) {
				continue
			}
			for _, dp := range parentDes {
				if m.isAbsent(dp.MorphoNum) {
					continue
				}
				dc := cloneDesinence(dp, m)
				m.Desinences[mn] = append(m.Desinences[mn], dc)
				l.addDesinence(dc)
			}
		}

		// Inherit radical rules
		for _, d := range m.AllDesinences() {
			if _, ok := m.RadicalRules[d.RadNum]; !ok {
				if rule, ok := m.parent.RadicalRules[d.RadNum]; ok {
					m.RadicalRules[d.RadNum] = rule
				}
			}
		}

		// Inherit absents
		m.Absents = m.parent.Absents
	}

	// Apply suffixes collected from "suf" directives
	var sufDesSlice []*Desinence
	for _, se := range sufEntries {
		// find current desinences for this morpho
		for _, d := range m.Desinences[se.morpho] {
			grq := d.Grq + se.suf
			ds := &Desinence{
				Grq:       grq,
				Gr:        Atone(grq),
				MorphoNum: d.MorphoNum,
				RadNum:    d.RadNum,
				Model:     m,
			}
			sufDesSlice = append(sufDesSlice, ds)
		}
	}
	for _, d := range sufDesSlice {
		m.Desinences[d.MorphoNum] = append(m.Desinences[d.MorphoNum], d)
		l.addDesinence(d)
	}

	if m.Name == "" {
		return nil
	}
	return m
}

// reVarRef matches a $variable reference with an optional prefix after a : or ;.
// Mirrors the C++ regex: [:;]([\w]*)\+{0,1}(\$\w+)
// Go's \w is ASCII-only, so we use \pL (Unicode letter) to match Latin characters
// with diacritics (e.g., ānd, īssĭm, ūr) that appear as prefixes in modeles.la.
var reVarRef = regexp.MustCompile(`[:;]([0-9\pL]*)\+?(\$\w+)`)

// substituteVars replaces $variable references in line with their stored values.
// Mirrors the variable substitution loop in Modele::Modele: when a variable has
// a prefix (e.g., "ānd$lupus"), the prefix is inserted after each semicolon in
// the variable's value so it applies to every element.
func (l *Lemmatizer) substituteVars(line string) string {
	for reVarRef.MatchString(line) {
		m := reVarRef.FindStringSubmatch(line)
		varName := m[2]
		val, ok := l.variables[varName]
		if !ok {
			break
		}
		pre := m[1]
		if pre != "" {
			val = strings.Replace(val, ";", ";"+pre, -1)
		}
		line = strings.Replace(line, varName, val, 1)
	}
	return line
}

// loadLexicon reads bin/data/lemmes.la and builds l.lemmas and l.radicals.
// Mirrors Lemmat::lisLexique.
func (l *Lemmatizer) loadLexicon(dataDir string) error {
	return l.loadLexiconFile(filepath.Join(dataDir, "lemmes.la"), false)
}

// loadExtendedLexicon reads bin/data/lem_ext.la if present.
// Same format as lemmes.la but entries do not overwrite existing lemmas.
// Lemmas loaded here are marked as Extended.
func (l *Lemmatizer) loadExtendedLexicon(dataDir string) error {
	path := filepath.Join(dataDir, "lem_ext.la")
	if _, err := os.Stat(path); err != nil {
		return nil // file not present, not an error
	}
	return l.loadLexiconFile(path, true)
}

// loadLexiconFile reads a single lemmes.la-format file and registers
// its lemmas and radicals. Existing lemmas are not overwritten.
func (l *Lemmatizer) loadLexiconFile(path string, extended bool) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", filepath.Base(path), err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}

		lemma := newLemma(line)
		if lemma == nil {
			continue
		}
		lemma.Extended = extended

		// Do not overwrite existing lemmas
		if _, exists := l.lemmas[lemma.Key]; exists {
			continue
		}

		// Resolve model
		lemma.model = l.models[lemma.modelName]
		if lemma.model != nil && lemma.POS == POSUnknown {
			lemma.POS = lemma.model.POS()
		}

		l.lemmas[lemma.Key] = lemma

		// Build and register radicals
		l.buildRadicals(lemma)
	}
	return sc.Err()
}

// stemFromGrq computes the stem string from a canonical form (grq) and a radical
// rule string ("K", "n", or "n,suffix"), mirroring the C++ radical derivation.
func stemFromGrq(grq, rule string) string {
	// Strip trailing combining breve if present
	grq = strings.TrimSuffix(grq, "\u0306")
	if rule == "K" {
		return grq
	}
	ruleParts := strings.SplitN(rule, ",", 2)
	oter, _ := strconv.Atoi(ruleParts[0])
	runes := []rune(grq)
	if oter > len(runes) {
		oter = len(runes)
	}
	stem := string(runes[:len(runes)-oter])
	if len(ruleParts) > 1 && ruleParts[1] != "0" {
		stem += ruleParts[1]
	}
	return stem
}

// buildRadicals computes all radicals for a lemma from its model's radical rules,
// then registers them in the global radicals map.
// Mirrors Lemmat::ajRadicaux.
func (l *Lemmatizer) buildRadicals(lemma *Lemma) {
	m := lemma.model
	if m == nil {
		return
	}

	// First register explicit radicals already parsed from lemmes.la
	for _, rads := range lemma.radicals {
		for _, r := range rads {
			l.addRadical(r)
		}
	}

	// Then compute radicals from the model's radical rules (skip if already explicit)
	for rn, rule := range m.RadicalRules {
		if _, exists := lemma.radicals[rn]; exists {
			continue
		}

		// Iterate over the primary form and all alternative canonical forms,
		// matching the C++ ajRadicaux which calls l->grq().split(',') and
		// registers each derived radical on both the lemma and the global map.
		for _, grqForm := range append([]string{lemma.Grq}, lemma.altGrqs...) {
			stem := stemFromGrq(grqForm, rule)
			r := &Radical{
				Grq:   Communes(stem),
				Gr:    Atone(stem),
				Num:   rn,
				Lemma: lemma,
			}
			lemma.radicals[rn] = append(lemma.radicals[rn], r)
			l.addRadical(r)
		}
	}
}

// loadTranslations reads all lemmes.XX and lem_ext.XX files from dataDir.
// Mirrors Lemmat::lisTraductions.
func (l *Lemmatizer) loadTranslations(dataDir string) error {
	for _, pattern := range []string{"lemmes.*", "lem_ext.*"} {
		matches, err := filepath.Glob(filepath.Join(dataDir, pattern))
		if err != nil {
			return err
		}
		for _, path := range matches {
			ext := filepath.Ext(path)
			if ext == ".la" || ext == "" {
				continue
			}
			lang := ext[1:] // strip leading "."
			if err := l.loadTranslationFile(path, lang); err != nil {
				// Non-fatal: skip missing/malformed files
				continue
			}
		}
	}
	return nil
}

// loadTranslationFile reads a single lemmes.XX file.
// New format: first non-comment non-empty line is the language name (bare, no ! prefix).
func (l *Lemmatizer) loadTranslationFile(path, lang string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	langNameSet := false

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}
		// First non-comment line is the language name (no colon → it's not a lemma entry)
		if !langNameSet {
			if !strings.Contains(line, ":") {
				l.languages[lang] = line
				langNameSet = true
				continue
			}
			// If it has a colon it might be an old-format file; treat as language name anyway
			l.languages[lang] = lang
			langNameSet = true
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := Deramise(line[:idx])
		translation := line[idx+1:]
		if lemma := l.lemmas[key]; lemma != nil {
			lemma.AddTranslation(lang, translation)
		}
	}
	return sc.Err()
}

// loadIrregs reads bin/data/irregs.la and populates l.irregs.
// Mirrors Lemmat::lisIrreguliers.
func (l *Lemmatizer) loadIrregs(dataDir string) error {
	f, err := os.Open(filepath.Join(dataDir, "irregs.la"))
	if err != nil {
		return fmt.Errorf("open irregs.la: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) < 3 {
			continue
		}

		grq := parts[0]
		exclusive := strings.HasSuffix(grq, "*")
		if exclusive {
			grq = grq[:len(grq)-1]
		}
		gr := Atone(grq)

		lemmaKey := Deramise(parts[1])
		lemma := l.lemmas[lemmaKey]
		if lemma == nil {
			continue
		}

		irr := &Irreg{
			Grq:       grq,
			Gr:        gr,
			Exclusive: exclusive,
			Lemma:     lemma,
			Morphos:   ListI(parts[2]),
		}

		key := Deramise(gr)
		l.irregs[key] = append(l.irregs[key], irr)
		lemma.addIrreg(irr)
	}
	return sc.Err()
}

// loadAssims reads data/assimilations.la and populates both the atone
// table (l.assims, used to probe the input) and the quantity-marked table
// (l.assimsq, used to rewrite marked result forms after an assim/desassim
// fallback). It also precomputes the longest-first iteration slices
// consumed by assim/desassim/assimq/desassimq.
// Mirrors LemCore::ajAssims.
func (l *Lemmatizer) loadAssims(dataDir string) error {
	f, err := os.Open(filepath.Join(dataDir, "assimilations.la"))
	if err != nil {
		return fmt.Errorf("open assimilations.la: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		keyQ := line[:idx]
		valQ := line[idx+1:]
		l.assimsq[keyQ] = valQ
		l.assims[Atone(keyQ)] = Atone(valQ)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	l.buildAssimIterOrder()
	return nil
}

// buildAssimIterOrder precomputes the longest-first iteration orders for
// both the atone assim table and the quantity-marked assimsq table.
// Without a deterministic longest-first order, Go's randomized map
// iteration could pick a shorter prefix (e.g. "ads") before a longer
// one (e.g. "adst"), producing non-deterministic — and semantically
// wrong — assimilation results. Ties are broken lexicographically so
// the final order is fully stable.
func (l *Lemmatizer) buildAssimIterOrder() {
	l.assimsByKey, l.assimsByVal = sortedAssimEntries(l.assims)
	l.assimsqByKey, l.assimsqByVal = sortedAssimEntries(l.assimsq)
}

// sortedAssimEntries returns two views of m sorted longest-first: byKey
// is ordered by len(key) desc (used to match unassimilated prefixes),
// byVal by len(val) desc (used to match assimilated prefixes). Ties are
// broken lexicographically on the relevant field.
func sortedAssimEntries(m map[string]string) (byKey, byVal []assimEntry) {
	byKey = make([]assimEntry, 0, len(m))
	for k, v := range m {
		byKey = append(byKey, assimEntry{key: k, val: v})
	}
	byVal = append([]assimEntry(nil), byKey...)
	sort.Slice(byKey, func(i, j int) bool {
		a, b := byKey[i], byKey[j]
		if len(a.key) != len(b.key) {
			return len(a.key) > len(b.key)
		}
		return a.key < b.key
	})
	sort.Slice(byVal, func(i, j int) bool {
		a, b := byVal[i], byVal[j]
		if len(a.val) != len(b.val) {
			return len(a.val) > len(b.val)
		}
		return a.val < b.val
	})
	return byKey, byVal
}

// loadContractions reads data/contractions.la and populates l.contractions.
// Format: "key:value" (without quantity marks).
// Mirrors LemCore::ajContractions.
func (l *Lemmatizer) loadContractions(dataDir string) error {
	f, err := os.Open(filepath.Join(dataDir, "contractions.la"))
	if err != nil {
		return fmt.Errorf("open contractions.la: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		l.contractions[line[:idx]] = line[idx+1:]
	}
	return sc.Err()
}
