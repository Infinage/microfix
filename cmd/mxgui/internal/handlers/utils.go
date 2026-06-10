package gui

import (
	"fmt"
	"strings"

	"github.com/infinage/microfix/pkg/spec"
)

type FieldInfo struct {
	Tag uint16
	Name string
	Required string
	SampleValues string
}

// TODO: Flatten the entry
func flattenMessageSpec(entry spec.Entry, sp *spec.Spec) ([]FieldInfo, error) {
	var result []FieldInfo	
	for _, en := range entry.Entries {
		tag, ok := sp.FieldNames[en.Name]
		if !ok {
			return nil, fmt.Errorf("missing field name [%s]", en.Name)
		}

		fDef, ok := sp.Fields[tag]
		if !ok {
			return nil, fmt.Errorf("missing field def for tag [%d]", tag)
		}

		reqStr := "N"
		if en.Required {
			reqStr = "Y"
		}

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

		result = append(result, FieldInfo{
			Tag: tag, 
			Name: fDef.Name, 
			Required: reqStr, 
			SampleValues: sampleValuesBuilder.String(),
		})
	}

	return result, nil
}
