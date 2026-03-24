# Collatinus — Go library for Latin morphological analysis

A pure Go library for lemmatization and morphological analysis of Latin text,
derived from [Collatinus-11](https://github.com/biblissima/collatinus) (C++/Qt).
It parses the same data files and implements the same algorithms, with no Qt
dependency.

## Features

- **Lemmatization** — given a Latin word form, find the dictionary headword(s)
  and morphological description (case, number, tense, mood, etc.)
- **Inflection tables** — generate the complete paradigm for any lemma
- **Translations** — multilingual definitions (fr, de, en, es, it, pt, …)
- **Enclitic stripping** — handles *-que*, *-ne*, *-ue*, *-ve*, *-st* automatically
- **Assimilation / contraction** — recognises assimilated and contracted perfect forms

## Data files

The `data/` directory contains the linguistic data originally distributed with
Collatinus-11:

| File | Content |
|------|---------|
| `morphos.fr` / `morphos.en` / `morphos.es` | 416 morphological descriptions (1-based) |
| `modeles.la` | 141 inflection models |
| `lemmes.la` | ~24 000 Latin headwords with radical rules and occurrence counts |
| `lemmes.fr/de/en/…` | Multilingual translations |
| `irregs.la` | Irregular forms |
| `assimilations.la` | Prefix-assimilation table (with quantity marks) |
| `contractions.la` | Perfect-contraction expansion table |
| `abreviations.la` | Abbreviation list |
| `parpos.txt` | Vowel-quantity rules by position |

## Installation

```
go get github.com/cours-de-latin/collatinus
```

## Quick start

```go
package main

import (
    "fmt"
    "github.com/cours-de-latin/collatinus"
)

func main() {
    l, err := collatinus.New("data")
    if err != nil {
        panic(err)
    }

    // Lemmatize a word form
    results := l.LemmatizeWord("puellae", false)
    for lemma, analyses := range results {
        fmt.Printf("%s (%s)\n", lemma.Grq, lemma.Translation("fr"))
        for _, a := range analyses {
            fmt.Printf("  %s\n", a.MorphoDescription)
        }
    }
    // Output:
    // pŭēlla (fille, jeune fille)
    //   génitif singulier
    //   datif singulier
    //   nominatif pluriel
    //   vocatif pluriel

    // Full inflection table
    table := l.InflectionTable(l.Lemma("lupus"))
    for mn, forms := range table.Cells {
        fmt.Printf("%s: %v\n", l.Morpho(mn), forms)
    }

    // Lemmatize a full text
    for _, r := range l.LemmatizeText("Gallia est omnis divisa in partes tres") {
        fmt.Printf("%s → %d lemma(s)\n", r.Token, len(r.Analyses))
    }
}
```

## API

```go
// Load data
func New(dataDir string) (*Lemmatizer, error)

// Lemmatization
func (l *Lemmatizer) LemmatizeWord(form string, sentenceStart bool) map[*Lemma][]Analysis
func (l *Lemmatizer) LemmatizeText(text string) []LemmatizationResult

// Lookup
func (l *Lemmatizer) Lemma(key string) *Lemma
func (l *Lemmatizer) Morpho(index int) string               // French descriptions
func (l *Lemmatizer) MorphoLang(index int, lang string) string // any loaded language, falls back to French
func (l *Lemmatizer) MorphoLanguages() []string              // e.g. ["fr","en","es","k9"]
func (l *Lemmatizer) InflectionTable(lemma *Lemma) *InflectionTable
func (l *Lemmatizer) Languages() map[string]string           // translation languages

// Lemma
func (l *Lemma) Translation(lang string) string
func (l *Lemma) Model() *Model

// Result types
type Analysis struct {
    FormWithMarks     string // form with vowel-quantity marks
    MorphoDescription string // e.g. "nominatif singulier"
    MorphoIndex       int
}
type LemmatizationResult struct {
    Token    string
    Analyses map[*Lemma][]Analysis
}
type InflectionTable struct {
    Lemma *Lemma
    Cells map[int][]string // morphoIndex → []form
}
```

## Provenance and licence

The linguistic data (`data/`) and the algorithms implemented in this library
originate from **Collatinus-11** by Yves Ouvrard and Philippe Verkerk,
distributed under the [GNU General Public Licence v2](https://www.gnu.org/licenses/old-licenses/gpl-2.0.html)
(or later). This Go library is therefore also released under the **GPL v2+**.
