package mfix

import (
	"embed"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
)

// BoolYN handles the FIX 'Y'/'N' attribute mapping
type boolYN bool

func (b *boolYN) UnmarshalXMLAttr(attr xml.Attr) error {
	*b = boolYN(attr.Value == "Y")
	return nil
}

// Entry represents any item inside a message, component or group.
type specEntry struct {
	XMLName  xml.Name
	Name     string      `xml:"name,attr"`
	Required boolYN      `xml:"required,attr"`
	Entries  []specEntry `xml:",any"`
}

// Struct for `<messages><message/></messages>`
type messageDef struct {
	Name    string      `xml:"name,attr"`
	MsgType string      `xml:"msgtype,attr"`
	Entries []specEntry `xml:",any"`
}

// Struct for `<components><component/><components>`
type componentDef struct {
	Name    string      `xml:"name,attr"`
	Entries []specEntry `xml:",any"`
}

// Holding header and trailer entries
type container struct {
	Entries []specEntry `xml:",any"`
}

// RawSpec matches spec.xml exactly
type rawSpec struct {
	Type       string         `xml:"type,attr"`
	Major      int            `xml:"major,attr"`
	Minor      int            `xml:"minor,attr"`
	Sp         int            `xml:"servicepack,attr"`
	Header     container      `xml:"header"`
	Trailer    container      `xml:"trailer"`
	Messages   []messageDef   `xml:"messages>message"`
	Components []componentDef `xml:"components>component"`
	Fields     []FieldDef     `xml:"fields>field"`
}

// Struct for `<fields><field/><field>`
type FieldDef struct {
	Number int    `xml:"number,attr"`
	Name   string `xml:"name,attr"`
	Type   string `xml:"type,attr"`
	Enums  []struct {
		Enum        string `xml:"enum,attr"`
		Description string `xml:"description,attr"`
	} `xml:"value"`
}

//go:embed spec/*.xml
var defaultSpecs embed.FS

// Parse the XML spec and return a faithful object representation
func loadRawSpec(path string) (rawSpec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		raw, err = defaultSpecs.ReadFile(filepath.Join("spec", path))
		if err != nil {
			return rawSpec{}, fmt.Errorf("Could not find spec %s in local or embedded path", path)
		}
	}

	// Load into raw struct
	var data rawSpec
	err = xml.Unmarshal(raw, &data)
	if err != nil {
		return rawSpec{}, err
	}

	return data, nil
}
