package pretty

import (
	"bytes"
	"strings"
	"testing"

	"github.com/infinage/microfix/pkg/spec"
)

func TestWritePrettyFieldDef(t *testing.T) {
	field := spec.FieldDef{
		Name: "MsgType",
		Type: "STRING",
		Enums: []spec.EnumDef{
			{Enum: "0", Description: "HEARTBEAT"},
			{Enum: "A", Description: "LOGON"},
		},
	}

	var buf bytes.Buffer
	WritePrettyFieldDef(&buf, field)
	output := buf.String()

	// Verify header
	if !strings.Contains(output, "Field: MsgType") {
		t.Errorf("Expected output to contain field name, got:\n%s", output)
	}

	// Verify Enums (Check for exact formatting)
	if !strings.Contains(output, "  0     -> HEARTBEAT") {
		t.Errorf("Enum formatting mismatch, got:\n%s", output)
	}
}
