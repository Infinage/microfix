package inspector

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/infinage/microfix/pkg/message"
	"github.com/infinage/microfix/pkg/spec"
)

// --- Global Router Cache ---
var (
	cachedRouters = make(map[string]*spec.Router)
	routerMutex   sync.Mutex
)

// getTestRouter loads a router once per version and reuses it for all subsequent test calls.
func getTestRouter(t *testing.T, version string) *spec.Router {
	routerMutex.Lock()
	defer routerMutex.Unlock()

	if r, ok := cachedRouters[version]; ok {
		return r
	}

	r, err := spec.NewDefaultRouter(version)
	if err != nil {
		t.Fatalf("Failed to load router %s: %v", version, err)
	}
	cachedRouters[version] = r
	return r
}

// --- Mocks ---
func fieldFn(fields map[uint16]spec.FieldDef) func(uint16) (spec.FieldDef, bool) {
	return func(u uint16) (spec.FieldDef, bool) {
		fDef, ok := fields[u]
		return fDef, ok
	}
}

// --- Tests ---

func TestWalkSpecBasic_ParsesUntilEnd(t *testing.T) {
	router := getTestRouter(t, "FIX44")

	// Message: 11=ID1, 39=0 (Enum for New)
	msg := message.Message{{Tag: 11, Value: "ID1"}, {Tag: 39, Value: "0"}}

	if pos, nodes := walkSpecBasic(&msg, 0, router.Field, nil); pos != 2 {
		t.Fatalf("Expected to parse all 2 fields, stopped at index %d", pos)
	} else if len(nodes) != 2 {
		t.Fatalf("Expected 2 nodes, got %d", len(nodes))
	} else if nodes[0].Name != "ClOrdID" {
		t.Errorf("Expected Tag 11 to resolve to ClOrdID, got '%s'", nodes[0].Name)
	} else if nodes[1].Name != "OrdStatus" || nodes[1].EnumDesc != "NEW" {
		t.Errorf("Expected Tag 39 to resolve to OrdStatus (New), got '%s (%s)'",
			nodes[1].Name, nodes[1].EnumDesc)
	}
}

func TestWalkSpecBasic_StopsOnNotInContext(t *testing.T) {
	router := getTestRouter(t, "FIX44")
	notInContext := map[uint16]int{10: 0}

	msg := message.Message{
		{Tag: 11, Value: "ID1"},
		{Tag: 37, Value: "ORDER1"},
		{Tag: 10, Value: "092"},
		{Tag: 39, Value: "0"},
	}

	if pos, nodes := walkSpecBasic(&msg, 0, router.Field, notInContext); pos != 2 {
		t.Fatalf("Expected to stop exactly at index 2, but stopped at %d", pos)
	} else if len(nodes) != 2 {
		t.Fatalf("Expected exactly 2 nodes parsed before hitting out-of-context tag, got %d", len(nodes))
	} else if nodes[1].Tag != 37 {
		t.Errorf("Expected the last parsed node to be Tag 37, got Tag %d", nodes[1].Tag)
	}
}

func TestWalkSpecBasic_HandlesUnknownTags(t *testing.T) {
	router := getTestRouter(t, "FIX44")

	msg := message.Message{{Tag: 11, Value: "ID1"}, {Tag: 9999, Value: "CUSTOM_DATA"}}

	if pos, nodes := walkSpecBasic(&msg, 0, router.Field, nil); pos != 2 {
		t.Fatalf("Expected to parse all 2 fields, stopped at index %d", pos)
	} else if len(nodes) != 2 {
		t.Fatalf("Expected 2 nodes, got %d", len(nodes))
	} else if unknownNode := nodes[1]; unknownNode.Tag != 9999 || unknownNode.Value != "CUSTOM_DATA" {
		t.Errorf("Expected node for 9999=CUSTOM_DATA, got %d=%s", unknownNode.Tag, unknownNode.Value)
	} else if unknownNode.Name != "" || unknownNode.EnumDesc != "" {
		t.Errorf("Expected unknown tag to have empty Name/EnumDesc, got Name='%s' EnumDesc='%s'",
			unknownNode.Name, unknownNode.EnumDesc)
	}
}

func TestWalkSpec_StandardFields(t *testing.T) {
	fields := map[uint16]spec.FieldDef{
		35: {Name: "MsgType", Enums: []spec.EnumDef{{Enum: "A", Description: "Logon"}}},
		49: {Name: "SenderCompID"},
		56: {Name: "TargetCompID"},
		10: {Name: "CheckSum"},
	}

	context := spec.Entry{
		Lookup:  map[uint16]int{35: 0, 49: 1, 56: 2},
		Entries: []spec.Entry{{Name: "MsgType"}, {Name: "SenderCompID"}, {Name: "TargetCompID"}},
	}

	msg := message.Message{
		{Tag: 35, Value: "A"},
		{Tag: 49, Value: "CLIENT"},
		{Tag: 56, Value: "SERVER"},
		{Tag: 10, Value: "092"},
	}

	if pos, nodes := walkSpec(&msg, 0, context, nil, fieldFn(fields)); pos != 3 {
		t.Fatalf("Expected to stop at index 3 (Tag 10), but stopped at %d", pos)
	} else if len(nodes) != 3 {
		t.Fatalf("Expected 3 parsed nodes, got %d", len(nodes))
	} else if nodes[0].Tag != 35 || nodes[0].EnumDesc != "Logon" {
		t.Errorf("Expected Tag 35 to resolve to Logon, got %s", nodes[0].EnumDesc)
	}
}

func TestWalkSpec_RepeatingGroup(t *testing.T) {
	fields := map[uint16]spec.FieldDef{
		268: {Name: "NoMDEntries"},
		269: {Name: "MDEntryType", Enums: []spec.EnumDef{{Enum: "0", Description: "Bid"}, {Enum: "1", Description: "Ask"}}},
		270: {Name: "MDEntryPx"},
		10:  {Name: "CheckSum"},
	}

	groupContext := spec.Entry{
		Lookup:  map[uint16]int{269: 0, 270: 1},
		Entries: []spec.Entry{{Name: "MDEntryType"}, {Name: "MDEntryPx"}},
	}
	mainContext := spec.Entry{
		Lookup: map[uint16]int{268: 0},
		Entries: []spec.Entry{
			{
				Name:    "NoMDEntries",
				IsGroup: true,
				Lookup:  groupContext.Lookup,
				Entries: groupContext.Entries,
			},
		},
	}

	msg := message.Message{
		{Tag: 268, Value: "2"},
		{Tag: 269, Value: "0"},
		{Tag: 270, Value: "150.50"},
		{Tag: 269, Value: "1"},
		{Tag: 270, Value: "151.20"},
		{Tag: 10, Value: "092"},
	}

	pos, nodes := walkSpec(&msg, 0, mainContext, nil, fieldFn(fields))
	if pos != 5 {
		t.Fatalf("Expected to stop at index 5, but stopped at %d", pos)
	} else if len(nodes) != 1 {
		t.Fatalf("Expected 1 top-level node (NoMDEntries), got %d", len(nodes))
	} else if !nodes[0].IsGroup {
		t.Fatal("Expected NoMDEntries node to be flagged as IsGroup=true")
	} else if size := len(nodes[0].Children); size != 2 {
		t.Fatalf("Expected 2 repeating entries, got %d", size)
	}

	groupNode := nodes[0]
	if groupNode.Children[1][0].Tag != 269 || groupNode.Children[1][0].EnumDesc != "Ask" {
		t.Errorf("Expected second entry's first tag to be 269 Ask, got Tag %d %s",
			groupNode.Children[1][0].Tag, groupNode.Children[1][0].EnumDesc)
	}
}

func TestWalkSpec_PreventsMutationAndInfiniteLoops(t *testing.T) {
	fields := map[uint16]spec.FieldDef{11: {Name: "ClOrdID"}}
	context := spec.Entry{
		Lookup:  map[uint16]int{11: 0},
		Entries: []spec.Entry{{Name: "ClOrdID"}},
	}

	msg := message.Message{{Tag: 11, Value: "ID_1"}, {Tag: 11, Value: "ID_2"}}

	if pos, nodes := walkSpec(&msg, 0, context, nil, fieldFn(fields)); pos != 1 {
		t.Fatalf("Expected parser to stop at index 1 due to duplicate tag, stopped at %d", pos)
	} else if len(nodes) != 1 {
		t.Fatalf("Expected exactly 1 node parsed, got %d", len(nodes))
	} else if _, exists := context.Lookup[11]; !exists {
		t.Fatal("CRITICAL: Global context.Lookup map was mutated! Tag 11 was deleted.")
	}
}

func TestFieldNode_JSON_Flat(t *testing.T) {
	node := FieldNode{Tag: 35, Value: "D", IsGroup: false}
	mp := make(map[uint16]any)
	node.json(&mp)
	if val, ok := mp[35]; !ok || val != "D" {
		t.Errorf("Expected map to contain 35='D', got %v", mp[35])
	}
}

func TestFieldNode_JSON_Group(t *testing.T) {
	node := FieldNode{
		Tag:     268,
		IsGroup: true,
		Children: [][]FieldNode{
			{{Tag: 269, Value: "0"}, {Tag: 270, Value: "150.50"}},
			{{Tag: 269, Value: "1"}, {Tag: 270, Value: "151.20"}},
		},
	}

	mp := make(map[uint16]any)
	node.json(&mp)

	if entries, ok := mp[268].([]map[uint16]any); !ok {
		t.Fatalf("Expected tag 268 to contain a slice of maps, got %T", mp[268])
	} else if len(entries) != 2 {
		t.Fatalf("Expected 2 group entries, got %d", len(entries))
	} else if entries[0][269] != "0" || entries[0][270] != "150.50" {
		t.Errorf("First entry is malformed: %v", entries[0])
	} else if entries[1][269] != "1" || entries[1][270] != "151.20" {
		t.Errorf("Second entry is malformed: %v", entries[1])
	}
}

func TestInspectView_JSON_FullMessage(t *testing.T) {
	view := InspectView{
		Header: []FieldNode{
			{Tag: 8, Value: "FIXT.1.1"},
			{Tag: 35, Value: "W"},
		},
		Body: []FieldNode{
			{Tag: 55, Value: "AAPL"},
			{Tag: 268, IsGroup: true, Children: [][]FieldNode{{{Tag: 269, Value: "0"}, {Tag: 270, Value: "150.00"}}}},
		},
		Trailer: []FieldNode{
			{Tag: 10, Value: "123"},
		},
		LeftOvers: []FieldNode{
			{Tag: 9999, Value: "CUSTOM"},
		},
	}

	resultMap := view.json()
	if resultMap[8] != "FIXT.1.1" || resultMap[55] != "AAPL" || resultMap[10] != "123" || resultMap[9999] != "CUSTOM" {
		t.Fatal("Failed to merge all sections (Header, Body, Trailer, Leftovers) into the root map")
	}

	jsonBytes, err := json.Marshal(resultMap)
	if err != nil {
		t.Fatalf("Failed to marshal map to JSON: %v", err)
	}

	jsonStr := string(jsonBytes)
	expectedStrings := []string{
		`"8":"FIXT.1.1"`,
		`"35":"W"`,
		`"55":"AAPL"`,
		`"268":[{"269":"0","270":"150.00"}]`,
		`"10":"123"`,
		`"9999":"CUSTOM"`,
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(jsonStr, expected) {
			t.Errorf("Final JSON string is missing expected substring: %s\nGot: %s", expected, jsonStr)
		}
	}
}

func TestInspectView_NoMemorySpike(t *testing.T) {
	router := getTestRouter(t, "FIXT11")

	testCases := []string{
		// Sequence Reset
		"8=FIXT.1.1|9=80|35=6|49=CLIENT|56=SERVER|34=2|52=20260618-06:28:16.226|23=STRING|28=N|54=1|27=S|10=011|",

		// New Order Single
		"8=FIXT.1.1|9=120|35=D|49=CLIENT|56=SERVER|34=3|52=20260618-06:30:00.000|11=ID1|21=1|55=AAPL|54=1|38=100|40=1|10=123|",

		// Execution Report with potential loops if parser fails
		"8=FIXT.1.1|9=130|35=8|49=SERVER|56=CLIENT|34=4|52=20260618-06:30:05.000|37=EXEC1|17=EXEC1|150=0|39=0|55=AAPL|54=1|151=100|14=0|10=045|",
	}

	for i, raw := range testCases {
		iview := NewInspectView(raw, "", router, spec.ValidationStrict)

		// Just verify the JSON compiles and the process doesn't hang or crash
		if iview.JSON == "" {
			t.Errorf("Test case %d failed to output JSON representation.", i)
		}

		// Log observations just to confirm standard behavior on malformed tags
		if !iview.IsValid {
			t.Logf("Case %d observations: %v", i, iview.Observations)
		}
	}
}

func TestWalkSpec_SoftBoundary_OOCBodyTags(t *testing.T) {
	// Define global dictionary
	fields := map[uint16]spec.FieldDef{
		11: {Name: "ClOrdID"},
		38: {Name: "OrderQty"},
		// Tag 1137 is globally known but won't be in our body context
		1137: {Name: "DefaultApplVerID"},
		// Tag 10 will act as our terminator (Trailer)
		10: {Name: "CheckSum"},
	}

	// Define our Body context (only expecting 11 and 38)
	bodyContext := spec.Entry{
		Lookup:  map[uint16]int{11: 0, 38: 1},
		Entries: []spec.Entry{{Name: "ClOrdID"}, {Name: "OrderQty"}},
	}

	// Define the Trailer tags as our soft-boundary terminator
	terminateOnlyOn := map[uint16]int{10: 0}

	msg := message.Message{
		{Tag: 11, Value: "ID1"},
		{Tag: 1137, Value: "9"}, // OOC Tag (Globally known, but not in Body)
		{Tag: 38, Value: "100"},
		{Tag: 10, Value: "092"}, // Terminator
	}

	if pos, nodes := walkSpec(&msg, 0, bodyContext, terminateOnlyOn, fieldFn(fields)); pos != 3 {
		// It should stop exactly at index 3 (Tag 10), leaving Tag 10 to be parsed by the Trailer walker.
		t.Fatalf("Expected parser to stop at index 3 (terminator), got %d", pos)
	} else if len(nodes) != 3 {
		// It should have successfully parsed Tag 11, Tag 1137, and Tag 38 into the Body section.
		t.Fatalf("Expected 3 parsed nodes in body, got %d", len(nodes))
	} else if nodes[1].Tag != 1137 {
		// Ensure the OOC tag was proudly included in the Body nodes instead of breaking the loop
		t.Errorf("Expected OOC tag 1137 to be parsed into body, got %d", nodes[1].Tag)
	} else if nodes[2].Tag != 38 {
		// Ensure it continued walking successfully after the OOC tag
		t.Errorf("Expected tag 38 to be parsed after OOC tag, got %d", nodes[2].Tag)
	}
}

func TestInspectView_Integration_OOCAndGroups(t *testing.T) {
	router := getTestRouter(t, "FIX44")

	// This is a Market Data Snapshot (MsgType=W).
	// We inject Tag 11 (ClOrdID), which is valid in FIX44 but OUT OF CONTEXT for a Market Data message.
	// We also include a repeating group (Tag 268) with 2 entries.
	raw := "8=FIX.4.4|9=100|35=W|49=SENDER|56=TARGET|34=1|55=AAPL|11=OOC_ID|268=2|269=0|270=150.00|269=1|270=151.00|10=123|"
	view := NewInspectView(raw, "", router, spec.ValidationNone)

	// Verify Body absorbed the OOC tag
	var foundOOC bool
	var foundGroup bool
	for _, node := range view.Body {
		if node.Tag == 11 {
			foundOOC = true
			if node.Value != "OOC_ID" {
				t.Errorf("Expected OOC tag value 'OOC_ID', got '%s'", node.Value)
			}
		}
		if node.Tag == 268 {
			foundGroup = true
			if !node.IsGroup {
				t.Error("Expected Tag 268 to be identified as a group")
			}
			if len(node.Children) != 2 {
				t.Errorf("Expected 2 repeating group entries, got %d", len(node.Children))
			}
		}
	}

	if !foundOOC {
		t.Error("Expected Tag 11 to be absorbed into Body as an OOC tag, but it wasn't.")
	}
	if !foundGroup {
		t.Error("Expected Tag 268 repeating group to be parsed in Body.")
	}

	// Verify Trailer successfully parsed AFTER the OOC tag and Group
	if len(view.Trailer) == 0 {
		t.Fatal("Trailer was completely skipped!")
	} else if view.Trailer[0].Tag != 10 {
		t.Errorf("Expected Trailer to start with Tag 10, got %d", view.Trailer[0].Tag)
	}

	// Verify LeftOvers is empty (everything fell into the correct buckets)
	if len(view.LeftOvers) > 0 {
		t.Errorf("Expected 0 leftovers, got %d (First leftover tag: %d)", len(view.LeftOvers), view.LeftOvers[0].Tag)
	}
}
