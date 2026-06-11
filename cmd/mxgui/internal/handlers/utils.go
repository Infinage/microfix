package gui

import (
	"fmt"
	"strings"

	"github.com/infinage/microfix/pkg/spec"
)

type FieldInfo struct {
	Tag          uint16
	Name         string
	Required     string
	SampleValues string
}

// Recursively iterates through message entries and converts to HTML friendly spec
func flattenMessageSpec(result *[]FieldInfo, entry spec.Entry, sp *spec.Spec, includeOptional bool) error {
	for _, en := range entry.Entries {
		if !en.Required && !includeOptional {
			continue
		}

		tag, ok := sp.FieldNames[en.Name]
		if !ok {
			return fmt.Errorf("missing field name [%s]", en.Name)
		}

		fDef, ok := sp.Fields[tag]
		if !ok {
			return fmt.Errorf("missing field def for tag [%d]", tag)
		}

		reqStr := "N"
		if en.Required {
			reqStr = "Y"
		}

		// Convert enum contents into user friendly string, eg: "Int (0=New, 1=Replace, 2=Cancel)"
		var sampleValuesBuilder strings.Builder
		sampleValuesBuilder.WriteString(strings.ToUpper(fDef.Type[:1]) + strings.ToLower(fDef.Type[1:]))
		if len(fDef.Enums) > 0 {
			sampleValuesBuilder.WriteString("(")
			for i, enum := range fDef.Enums {
				if i > 0 {
					sampleValuesBuilder.WriteString(",")
				}
				sampleValuesBuilder.WriteString(enum.Enum + "=" + enum.Description)
			}
			sampleValuesBuilder.WriteString(")")
		}

		*result = append(*result, FieldInfo{
			Tag:          tag,
			Name:         fDef.Name,
			Required:     reqStr,
			SampleValues: sampleValuesBuilder.String(),
		})

		// Recurse if entry is a group
		if en.IsGroup {
			flattenMessageSpec(result, en, sp, includeOptional)
		}

	}

	return nil
}
