package executor

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseJumpTable_Success(t *testing.T) {
	tests := []struct {
		name      string
		script    string
		wantJumps map[int]Jump
		wantInstr []Instruction
	}{
		{
			name: "Simple If-Endif",
			script: `
				if 1=1
					print Hello
				endif
			`,
			wantJumps: map[int]Jump{
				0: {TargetOnFalse: 2, TargetOnEnd: 2},
			},
			wantInstr: []Instruction{
				{Text: "1=1", LineNo: 2, Type: "if"},
				{Text: "print Hello", LineNo: 3, Type: ""},
				{Text: "", LineNo: 4, Type: "endif"},
			},
		},
		{
			name: "If-Elif-Else-Endif",
			script: `
				if 1=1
					print A
				elif 2=2
					print B
				else
					print C
				endif
			`,
			wantJumps: map[int]Jump{
				0: {TargetOnFalse: 2, TargetOnEnd: 6}, // if points to elif (2) and endif (6)
				2: {TargetOnFalse: 4, TargetOnEnd: 6}, // elif points to else (4) and endif (6)
				4: {TargetOnEnd: 6},                   // else points to endif (6)
			},
			wantInstr: []Instruction{
				{Text: "1=1", LineNo: 2, Type: "if"},
				{Text: "print A", LineNo: 3, Type: ""},
				{Text: "2=2", LineNo: 4, Type: "elif"},
				{Text: "print B", LineNo: 5, Type: ""},
				{Text: "", LineNo: 6, Type: "else"},
				{Text: "print C", LineNo: 7, Type: ""},
				{Text: "", LineNo: 8, Type: "endif"},
			},
		},
		{
			name: "While with Break",
			script: `
				while 1=1
					if 2=2
						break
					endif
				endwhile
			`,
			wantJumps: map[int]Jump{
				0: {TargetOnFalse: 4},                 // while -> endwhile on false
				1: {TargetOnFalse: 3, TargetOnEnd: 3}, // if -> endif
				2: {TargetOnEnd: 4},                   // break -> endwhile
				4: {TargetOnEnd: 0},                   // endwhile -> while
			},
			wantInstr: []Instruction{
				{Text: "1=1", LineNo: 2, Type: "while"},
				{Text: "2=2", LineNo: 3, Type: "if"},
				{Text: "", LineNo: 4, Type: "break"},
				{Text: "", LineNo: 5, Type: "endif"},
				{Text: "", LineNo: 6, Type: "endwhile"},
			},
		},
		{
			name: "Nested while with breaks",
			script: `
				while 1=1
					while 2=2
						break
					endwhile
						break
				endwhile
			`,
			wantJumps: map[int]Jump{
				0: {TargetOnFalse: 5}, // while -> endwhile on false
				1: {TargetOnFalse: 3}, // while -> endwhile on false
				2: {TargetOnEnd: 3},   // break -> endwhile
				3: {TargetOnEnd: 1},   // endwhile -> while
				4: {TargetOnEnd: 5},   // break -> endwhile
				5: {TargetOnEnd: 0},   // endwhile -> while
			},
			wantInstr: []Instruction{
				{Text: "1=1", LineNo: 2, Type: "while"},
				{Text: "2=2", LineNo: 3, Type: "while"},
				{Text: "", LineNo: 4, Type: "break"},
				{Text: "", LineNo: 5, Type: "endwhile"},
				{Text: "", LineNo: 6, Type: "break"},
				{Text: "", LineNo: 7, Type: "endwhile"},
			},
		},
		{
			name: "Conditionals with Exit",
			script: `
				if assert 1 == 1
				    exit
				endif
			`,
			wantJumps: map[int]Jump{
				0: {TargetOnFalse: 2, TargetOnEnd: 2},
			},
			wantInstr: []Instruction{
				{Text: "assert 1 == 1", LineNo: 2, Type: "if"},
				{Text: "", LineNo: 3, Type: "exit"},
				{Text: "", LineNo: 4, Type: "endif"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instructions, jumpTable, err := parseJumpTable(strings.NewReader(tt.script))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Assert Jumps
			if !reflect.DeepEqual(jumpTable, tt.wantJumps) {
				t.Errorf("jumpTable \ngot  = %v\nwant = %v", jumpTable, tt.wantJumps)
			}

			// Assert Instructions
			if !reflect.DeepEqual(instructions, tt.wantInstr) {
				t.Errorf("instructions \ngot  = %+v\nwant = %+v", instructions, tt.wantInstr)
			}
		})
	}
}

func TestParseJumpTable_SyntaxErrors(t *testing.T) {
	tests := []struct {
		name        string
		script      string
		errContains string
	}{
		{
			name: "Unclosed If",
			script: `
				if 1=1
					print Hello
			`,
			errContains: "unclosed 'if' block",
		},
		{
			name: "Break outside loop",
			script: `
				if 1=1
					break
				endif
			`,
			errContains: "'break' outside of a looping construct",
		},
		{
			name: "Double Else",
			script: `
				if 1=1
				else
				else
				endif
			`,
			errContains: "'else' block without a preceding if/elif",
		},
		{
			name: "Elif after Else",
			script: `
				if 1=1
				else
				elif 2=2
				endif
			`,
			errContains: "'elif' block without a preceding if/elif",
		},
		{
			name: "Token after keyword",
			script: `
				if 1=1
				else foo
				endif
			`,
			errContains: "unexpected token following 'else'",
		},
		{
			name: "Dangling Endwhile",
			script: `
				if 1=1
				endwhile
			`,
			errContains: "'endwhile' without a preceding while",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseJumpTable(strings.NewReader(tt.script))
			if err == nil {
				t.Fatalf("expected error containing '%s', got nil", tt.errContains)
			}
			if !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("error \ngot  = %v\nwant contains = %v", err.Error(), tt.errContains)
			}
		})
	}
}
