package spec

import (
	"embed"
	"encoding/xml"
	"fmt"
	"os"
	"path"
	"strings"
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

// Struct for `<field><value/></field>`
type EnumDef struct {
	Enum        string `xml:"enum,attr"`
	Description string `xml:"description,attr"`
}

// Struct for `<fields><field/><field>`
type FieldDef struct {
	Number int       `xml:"number,attr"`
	Name   string    `xml:"name,attr"`
	Type   string    `xml:"type,attr"`
	Enums  []EnumDef `xml:"value"`
}

//go:embed xml/*.xml
var defaultSpecs embed.FS

// Parse the XML spec and return a faithful object representation
func loadRawSpec(fpath string) (rawSpec, error) {
	raw, err := os.ReadFile(fpath)
	if err != nil {
		ext := ""
		if !strings.HasSuffix(fpath, ".xml") {
			ext = ".xml"
		}

		raw, err = defaultSpecs.ReadFile(path.Join("xml", fpath) + ext)
		if err != nil {
			return rawSpec{}, fmt.Errorf("Could not find spec %s in local or embedded path", fpath)
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

// Return true if given file is available on disk or on embed FS.
// Default entries: FIX40, FIX41, FIX42, FIX43, FIX44, FIX50, FIX50SP1, FIX50SP2, FIXT11
func CheckPath(specPath string) bool {
	if specPath == "" {
		return false
	}

	// Lookup for built-in specs
	builtIns := map[string]bool{
		"FIX40": true, "FIX41": true, "FIX42": true, "FIX43": true,
		"FIX44": true, "FIX50": true, "FIX50SP1": true, "FIX50SP2": true,
		"FIXT11": true,
	}

	// Strip .xml just in case the user typed "FIX44.xml"
	baseName := strings.TrimSuffix(specPath, ".xml")
	if builtIns[baseName] {
		return true
	}

	// Check the physical disk
	if info, err := os.Stat(specPath); err == nil {
		// Reject directories and non XML extensions
		if info.IsDir() || strings.ToLower(path.Ext(specPath)) != ".xml" {
			return false
		}
		return true
	}

	return false
}
