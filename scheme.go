package main

import (
	"context"
	"fmt"
	"sort"

	"gitlab.com/subins2000/govarnam/govarnamgo"
)

type matches struct {
	Exact       []string
	Possibility []string
}

type schemeDefinitionItem struct {
	Letter      string
	Category    string
	Exact       []string
	Possibility []string
}

type schemeDefinition struct {
	standardResponse
	Details     govarnamgo.SchemeDetails
	Definitions []schemeDefinitionItem
}

func getSchemeDetails(schemeID string) (govarnamgo.SchemeDetails, error) {
	var schemeInfo govarnamgo.SchemeDetails
	foundScheme := false
	for _, sd := range schemeDetails {
		if sd.Identifier == schemeID {
			schemeInfo = sd
			foundScheme = true
		}
	}

	if !foundScheme {
		return schemeInfo, fmt.Errorf("invalid scheme id")
	}

	return schemeInfo, nil
}

func getItemsFromSearchResults(ctx context.Context, searchResults []govarnamgo.Symbol) map[string]matches {
	items := make(map[string]matches)

	for _, r := range searchResults {
		exact := []string{}
		possibility := []string{}

		if r.MatchType == 1 {
			exact = []string{r.Pattern}
		} else {
			possibility = []string{r.Pattern}
		}

		item, ok := items[r.Value1]
		if ok {
			item.Exact = append(item.Exact, exact...)
			item.Possibility = append(item.Possibility, possibility...)
			items[r.Value1] = item
		} else {
			items[r.Value1] = matches{exact, possibility}
		}
	}

	return items
}

func getSchemeDefinitions(ctx context.Context, sd govarnamgo.SchemeDetails) ([]schemeDefinitionItem, error) {
	schemeID := sd.Identifier

	// Vowels
	var symbol govarnamgo.Symbol
	// TODO use constant value from govarnam instead of hardcode
	symbol.Type = 1 // Vowel
	searchResultsI, _ := searchSymbolTable(ctx, schemeID, symbol)
	searchResults := searchResultsI.([]govarnamgo.Symbol)

	items := getItemsFromSearchResults(ctx, searchResults)

	// For sorting map
	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var categorizedResult []schemeDefinitionItem

	for _, letter := range keys {
		item := items[letter]
		categorizedResult = append(categorizedResult, schemeDefinitionItem{
			Letter:      letter,
			Category:    searchResults[0].Value1, // അ
			Exact:       item.Exact,
			Possibility: item.Possibility,
		})
	}

	// consonants
	if sd.LangCode == "ml" {
		categorizedResult = append(categorizedResult, getMLConsonants(ctx, sd)...)
	}

	// zwj, virama, other characters
	categorizedResult = append(categorizedResult, getOtherCharacters(ctx, sd)...)

	return categorizedResult, nil
}

func getMLConsonants(ctx context.Context, sd govarnamgo.SchemeDetails) []schemeDefinitionItem {
	letterSets := map[string][]string{
		"ക":          []string{"ക", "ഖ", "ഗ", "ഘ", "ങ"},
		"ച":          []string{"ച", "ഛ", "ജ", "ഝ", "ഞ"},
		"ട":          []string{"ട", "ഠ", "ഡ", "ഢ", "ണ"},
		"ത":          []string{"ത", "ഥ", "ദ", "ധ", "ന", "ഩ"},
		"പ":          []string{"പ", "ഫ", "ബ", "ഭ", "മ"},
		"യ":          []string{"യ", "ര", "ല", "വ", "ശ", "ഷ", "സ", "ഹ", "ള", "ഴ", "റ"},
		"ചില്ലക്ഷരം": []string{"ൻ", "ർ", "ൽ", "ൾ", "ൺ", "ൿ"},
	}

	items := make(map[string]matches)

	var symbol govarnamgo.Symbol
	symbol.Type = 2 // consonant
	searchResultsI, _ := searchSymbolTable(ctx, sd.Identifier, symbol)
	searchResults := searchResultsI.([]govarnamgo.Symbol)

	for _, r := range searchResults {
		for _, letterSet := range letterSets {
			for _, letter := range letterSet {
				if r.Value1 == letter {
					exact := []string{}
					possibility := []string{}

					if r.MatchType == 1 {
						exact = []string{r.Pattern}
					} else {
						possibility = []string{r.Pattern}
					}

					item, ok := items[r.Value1]
					if ok {
						item.Exact = append(item.Exact, exact...)
						item.Possibility = append(item.Possibility, possibility...)
						items[r.Value1] = item
					} else {
						items[r.Value1] = matches{exact, possibility}
					}
				}
			}
		}
	}

	var categorizedResult []schemeDefinitionItem
	for category, letterSet := range letterSets {
		for _, letter := range letterSet {
			categorizedResult = append(categorizedResult, schemeDefinitionItem{
				Letter:      letter,
				Category:    category,
				Exact:       items[letter].Exact,
				Possibility: items[letter].Possibility,
			})
		}
	}

	return categorizedResult
}

func getSchemeLetterDefinitions(ctx context.Context, sd govarnamgo.SchemeDetails, letter string) ([]schemeDefinitionItem, error) {
	items := make(map[string]matches)

	var symbol govarnamgo.Symbol
	symbol.Value1 = "LIKE " + letter + "%"
	searchResultsI, _ := searchSymbolTable(ctx, sd.Identifier, symbol)
	searchResults := searchResultsI.([]govarnamgo.Symbol)

	for _, r := range searchResults {
		exact := []string{}
		possibility := []string{}

		if r.MatchType == 1 {
			exact = []string{r.Pattern}
		} else {
			possibility = []string{r.Pattern}
		}

		item, ok := items[r.Value1]
		if ok {
			item.Exact = append(item.Exact, exact...)
			item.Possibility = append(item.Possibility, possibility...)
			items[r.Value1] = item
		} else {
			items[r.Value1] = matches{exact, possibility}
		}
	}

	var categorizedResult []schemeDefinitionItem
	for letterCombo, item := range items {
		letterComboRunes := []rune(letterCombo)
		category := letter
		if len(letterComboRunes) > 2 {
			category = string(letterComboRunes[2])
		}

		categorizedResult = append(categorizedResult, schemeDefinitionItem{
			Letter:      letterCombo,
			Category:    category,
			Exact:       item.Exact,
			Possibility: item.Possibility,
		})
	}

	return categorizedResult, nil
}

func getCategorizedFromSearchResults(ctx context.Context, searchResults []govarnamgo.Symbol, category string) []schemeDefinitionItem {
	var categorizedResult []schemeDefinitionItem
	items := getItemsFromSearchResults(ctx, searchResults)

	for letter, r := range items {
		if category == "" {
			category = letter
		}
		categorizedResult = append(categorizedResult, schemeDefinitionItem{
			Letter:      letter,
			Category:    category,
			Exact:       r.Exact,
			Possibility: r.Possibility,
		})
	}

	return categorizedResult
}

func getOtherCharacters(ctx context.Context, sd govarnamgo.SchemeDetails) []schemeDefinitionItem {
	var categorizedResult []schemeDefinitionItem

	i := 6        // Symbol
	for i <= 10 { // to Other symbols
		var symbol govarnamgo.Symbol
		symbol.Type = i
		searchResultsI, _ := searchSymbolTable(ctx, sd.Identifier, symbol)
		searchResults := searchResultsI.([]govarnamgo.Symbol)

		categorizedResult = append(categorizedResult, getCategorizedFromSearchResults(ctx, searchResults, "")...)

		i++
	}

	var symbol govarnamgo.Symbol
	symbol.Type = 11 // ZWNJ
	searchResultsI, _ := searchSymbolTable(ctx, sd.Identifier, symbol)
	searchResults := searchResultsI.([]govarnamgo.Symbol)

	categorizedResult = append(categorizedResult, getCategorizedFromSearchResults(ctx, searchResults, "ZWNJ - Zero Width Non Joiner")...)

	symbol.Type = 12 // ZWJ
	searchResultsI, _ = searchSymbolTable(ctx, sd.Identifier, symbol)
	searchResults = searchResultsI.([]govarnamgo.Symbol)

	categorizedResult = append(categorizedResult, getCategorizedFromSearchResults(ctx, searchResults, "ZWJ - Zero Width Joiner")...)

	symbol.Type = 13 // Period
	searchResultsI, _ = searchSymbolTable(ctx, sd.Identifier, symbol)
	searchResults = searchResultsI.([]govarnamgo.Symbol)

	categorizedResult = append(categorizedResult, getCategorizedFromSearchResults(ctx, searchResults, "")...)

	return categorizedResult
}
