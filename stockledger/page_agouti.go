package stockledger

import "github.com/sclevine/agouti"

// AgoutiSelection adapts an *agouti.Selection to ElementReader.
func AgoutiSelection(selection *agouti.Selection) ElementReader {
	if selection == nil {
		return nilSelection{}
	}
	return agoutiSelection{selection}
}

// AgoutiMultiSelection adapts an *agouti.MultiSelection to ElementReader.
func AgoutiMultiSelection(selection *agouti.MultiSelection) ElementReader {
	if selection == nil {
		return nilSelection{}
	}
	return agoutiMultiSelection{selection}
}

// AgoutiPage adapts an *agouti.Page to Page. The returned value reads the live
// DOM and never mutates the page.
func AgoutiPage(page *agouti.Page) Page {
	if page == nil {
		return nilPage{}
	}
	return agoutiPage{page}
}

type agoutiPage struct {
	page *agouti.Page
}

func (p agoutiPage) Find(selector string) ElementReader {
	return agoutiSelection{p.page.Find(selector)}
}

func (p agoutiPage) All(selector string) ElementReader {
	return agoutiMultiSelection{p.page.All(selector)}
}

type agoutiSelection struct {
	selection *agouti.Selection
}

func (s agoutiSelection) Attribute(name string) (string, error) {
	return s.selection.Attribute(name)
}

func (s agoutiSelection) Text() (string, error) {
	return s.selection.Text()
}

func (s agoutiSelection) Count() (int, error) {
	return s.selection.Count()
}

func (s agoutiSelection) At(index int) ElementReader {
	return nilSelection{}
}

func (s agoutiSelection) Find(selector string) ElementReader {
	return agoutiSelection{s.selection.Find(selector)}
}

func (s agoutiSelection) All(selector string) ElementReader {
	return agoutiMultiSelection{s.selection.All(selector)}
}

type agoutiMultiSelection struct {
	selection *agouti.MultiSelection
}

func (s agoutiMultiSelection) Attribute(name string) (string, error) {
	return s.selection.Attribute(name)
}

func (s agoutiMultiSelection) Text() (string, error) {
	return s.selection.Text()
}

func (s agoutiMultiSelection) Count() (int, error) {
	return s.selection.Count()
}

func (s agoutiMultiSelection) At(index int) ElementReader {
	return agoutiSelection{s.selection.At(index)}
}

func (s agoutiMultiSelection) Find(selector string) ElementReader {
	return agoutiSelection{s.selection.Find(selector)}
}

func (s agoutiMultiSelection) All(selector string) ElementReader {
	return agoutiMultiSelection{s.selection.All(selector)}
}

// nilSelection fails every read instead of panicking when a nil selection is
// adapted; ReadLedger reports it as ErrReadFailed.
type nilSelection struct{}

func (nilSelection) Attribute(name string) (string, error) {
	return "", errorf(ErrReadFailed, "nil selection cannot read %s", name)
}

func (nilSelection) Text() (string, error) {
	return "", errorf(ErrReadFailed, "nil selection cannot be read")
}

func (nilSelection) Count() (int, error) {
	return 0, errorf(ErrReadFailed, "nil selection cannot be counted")
}

func (nilSelection) At(index int) ElementReader { return nilSelection{} }

func (nilSelection) Find(selector string) ElementReader { return nilSelection{} }

func (nilSelection) All(selector string) ElementReader { return nilSelection{} }

type nilPage struct{}

func (nilPage) Find(selector string) ElementReader { return nilSelection{} }

func (nilPage) All(selector string) ElementReader { return nilSelection{} }
