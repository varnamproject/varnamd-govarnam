package libvarnam

// #cgo pkg-config: varnam
// #include <varnam.h>
import "C"
import "fmt"

// Varnam app binding.
type Varnam struct {
	handle *C.varnam
}

// VarnamError satisfies error interface.
type VarnamError struct {
	errorCode int
	message   string
}

// SchemeDetails returns language and other changes.
type SchemeDetails struct {
	LangCode     string
	Identifier   string
	DisplayName  string
	Author       string
	CompiledDate string
	IsStable     bool
}

// CorpusDetails returns corpus details.
type CorpusDetails struct {
	WordsCount int `json:"wordsCount"`
}

// LearnStatus .
type LearnStatus struct {
	TotalWords int
	Failed     int
}

func (e *VarnamError) Error() string {
	return e.message
}

// GetSuggestionsFilePath returns suggestions.
func (v *Varnam) GetSuggestionsFilePath() string {
	return C.GoString(C.varnam_get_suggestions_file(v.handle))
}

// GetSchemeFilePath returns the scheme file (.vst)
func (v *Varnam) GetSchemeFilePath() string {
	return C.GoString(C.varnam_get_scheme_file(v.handle))
}

// GetCorpusDetails will return corpus details.
func (v *Varnam) GetCorpusDetails() (*CorpusDetails, error) {
	var details *C.vcorpus_details

	rc := C.varnam_get_corpus_details(v.handle, &details)

	if rc != C.VARNAM_SUCCESS {
		errorCode := (int)(rc)

		return nil, &VarnamError{errorCode: errorCode, message: v.getVarnamError(errorCode)}
	}

	return &CorpusDetails{WordsCount: int(details.wordsCount)}, nil
}

// Transliterate given string to corresponding language.
func (v *Varnam) Transliterate(text string) ([]string, error) {
	var va *C.varray
	rc := C.varnam_transliterate(v.handle, C.CString(text), &va)

	if rc != C.VARNAM_SUCCESS {
		errorCode := (int)(rc)

		return nil, &VarnamError{errorCode: errorCode, message: v.getVarnamError(errorCode)}
	}

	var (
		i     C.int
		words []string
	)

	for i = 0; i < C.varray_length(va); i++ {
		word := (*C.vword)(C.varray_get(va, i))
		words = append(words, C.GoString(word.text))
	}

	return words, nil
}

// ReverseTransliterate given string.
func (v *Varnam) ReverseTransliterate(text string) (string, error) {
	var output *C.char
	rc := C.varnam_reverse_transliterate(v.handle, C.CString(text), &output)

	if rc != C.VARNAM_SUCCESS {
		errorCode := (int)(rc)

		return "", &VarnamError{errorCode: errorCode, message: v.getVarnamError(errorCode)}
	}

	return C.GoString(output), nil
}

// LearnFromFile learns from file from the given filepath.
func (v *Varnam) LearnFromFile(filePath string) (*LearnStatus, error) {
	var status C.vlearn_status
	rc := C.varnam_learn_from_file(v.handle, C.CString(filePath), &status, nil, nil)

	if rc != C.VARNAM_SUCCESS {
		errorCode := (int)(rc)

		return nil, &VarnamError{errorCode: errorCode, message: v.getVarnamError(errorCode)}
	}

	return &LearnStatus{TotalWords: int(status.total_words), Failed: int(status.failed)}, nil
}

// ImportFromFile Import learnigns from file (varnam exported file)
func (v *Varnam) ImportFromFile(filePath string) error {
	rc := C.varnam_import_learnings_from_file(v.handle, C.CString(filePath))

	if rc != C.VARNAM_SUCCESS {
		errorCode := (int)(rc)

		return &VarnamError{errorCode: errorCode, message: v.getVarnamError(errorCode)}
	}

	return nil
}

// Init initializes varnam bindings.
func Init(schemeIdentifier string) (*Varnam, error) {
	var (
		v   *C.varnam
		msg *C.char
	)

	rc := C.varnam_init_from_id(C.CString(schemeIdentifier), &v, &msg)

	if rc != C.VARNAM_SUCCESS {
		return nil, &VarnamError{errorCode: (int)(rc), message: C.GoString(msg)}
	}

	return &Varnam{handle: v}, nil
}

// GetAllSchemeDetails returns all scheme related details.
func GetAllSchemeDetails() []*SchemeDetails {
	allHandles := C.varnam_get_all_handles()

	if allHandles == nil {
		return []*SchemeDetails{}
	}

	var schemeDetails []*SchemeDetails

	length := int(C.varray_length(allHandles))

	for i := 0; i < length; i++ {
		var detail *C.vscheme_details

		handle := (*C.varnam)(C.varray_get(allHandles, C.int(i)))
		rc := C.varnam_get_scheme_details(handle, &detail)

		if rc != C.VARNAM_SUCCESS {
			return []*SchemeDetails{}
		}

		schemeDetails = append(schemeDetails, &SchemeDetails{
			LangCode: C.GoString(detail.langCode), Identifier: C.GoString(detail.identifier),
			DisplayName: C.GoString(detail.displayName), Author: C.GoString(detail.author),
			CompiledDate: C.GoString(detail.compiledDate), IsStable: detail.isStable > 0})

		C.varnam_destroy(handle)
	}

	return schemeDetails
}

// Learn from given input text.
func (v *Varnam) Learn(text string) error {
	rc := C.varnam_learn(v.handle, C.CString(text))

	if rc != 0 {
		errorCode := (int)(rc)

		return &VarnamError{errorCode: errorCode, message: v.getVarnamError(errorCode)}
	}

	return nil
}

// DeleteWord from given input text.
func (v *Varnam) DeleteWord(text string) error {
	rc := C.varnam_delete_word(v.handle, C.CString(text))

	if rc != 0 {
		errorCode := (int)(rc)

		return &VarnamError{errorCode: errorCode, message: v.getVarnamError(errorCode)}
	}

	return nil
}

func (v *Varnam) getVarnamError(errorCode int) string {
	errormessage := C.varnam_get_last_error(v.handle)
	varnamErrorMsg := C.GoString(errormessage)

	return fmt.Sprintf("%d:%s", errorCode, varnamErrorMsg)
}

// Destroy closes handle.
func (v *Varnam) Destroy() {
	C.varnam_destroy(v.handle)
}

// Train methods trains with the word and pattern,eg: pattern=chrome,word=ക്രോം
func (v *Varnam) Train(pattern, word string) error {
	rc := C.varnam_train(v.handle, C.CString(pattern), C.CString(word))

	if rc != 0 {
		errorCode := (int)(rc)

		return &VarnamError{errorCode: errorCode, message: v.getVarnamError(errorCode)}
	}

	return nil
}
