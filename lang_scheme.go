package main

import (
	"context"
	"fmt"

	"gitlab.com/subins2000/govarnam/govarnamgo"
)

type schemeDefinitionItem struct {
	Exact       []string
	Possibility []string
}

type schemeDefinition struct {
	standardResponse
	Details govarnamgo.SchemeDetails
	Def     []map[string]schemeDefinitionItem
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

func getLanguageSchemeDefinitions(ctx context.Context, sd govarnamgo.SchemeDetails) ([]map[string]schemeDefinitionItem, error) {
	var result []map[string]schemeDefinitionItem

	schemeID := sd.Identifier

	// Vowels
	var symbol govarnamgo.Symbol
	// TODO use constant value from govarnam instead of hardcode
	symbol.Type = 1 // Vowel
	searchResultsI, _ := searchSymbolTable(ctx, schemeID, symbol)
	searchResults := searchResultsI.([]govarnamgo.Symbol)

	if len(searchResults) > 0 {
		items := make(map[string]schemeDefinitionItem)
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
				items[r.Value1] = schemeDefinitionItem{exact, possibility}
			}
		}

		result = append(result, items)
	}

	// Consonants
	if sd.LangCode == "ml" {
		result = append(result, getMLNonVowels(ctx)...)
	}

	return result, nil
}

func getMLNonVowels(ctx context.Context) []map[string]schemeDefinitionItem {
	letterSets := [][]string{
		[]string{"ക", "ഖ", "ഗ", "ഘ", "ങ"},
		[]string{"ച", "ഛ", "ജ", "ഝ", "ഞ"},
		[]string{"ട", "ഠ", "ഡ", "ഢ", "ണ"},
		[]string{"ത", "ഥ", "ദ", "ധ", "ന", "ഩ"},
		[]string{"പ", "ഫ", "ബ", "ഭ", "മ"},
		[]string{"യ", "ര", "ല", "വ", "ശ", "ഷ", "സ", "ഹ", "ള", "ഴ", "റ"},
		[]string{"ൺ", "ൻ", "ർ", "ൽ", "ൾ", "ൿ"},
	}

	// 7 sets of letters
	const numberOfSets = 7

	items := make([]map[string]schemeDefinitionItem, numberOfSets)

	i := 0
	for i < numberOfSets {
		items[i] = make(map[string]schemeDefinitionItem)
		i++
	}

	var symbol govarnamgo.Symbol
	symbol.Type = 2 // Consonant
	searchResultsI, _ := searchSymbolTable(ctx, "ml", symbol)
	searchResults := searchResultsI.([]govarnamgo.Symbol)

	if len(searchResults) > 0 {
		for _, r := range searchResults {
			for i, consonantSet := range letterSets {
				for _, consonant := range consonantSet {
					if r.Value1 == consonant {
						exact := []string{}
						possibility := []string{}

						if r.MatchType == 1 {
							exact = []string{r.Pattern}
						} else {
							possibility = []string{r.Pattern}
						}

						item, ok := items[i][r.Value1]
						if ok {
							item.Exact = append(item.Exact, exact...)
							item.Possibility = append(item.Possibility, possibility...)
							items[i][r.Value1] = item
						} else {
							items[i][r.Value1] = schemeDefinitionItem{exact, possibility}
						}
					}
				}
			}
		}
	}

	return items
}
