package executor

import (
	"maps"
	"strings"
	"testing"
)

func TestParseJumpTable_Success(t *testing.T) {
	tests := []struct {
		name      string
		script    string
		wantJumps map[int]Jump
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, jumpTable, err := parseJumpTable(strings.NewReader(tt.script))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !maps.Equal(jumpTable, tt.wantJumps) {
				t.Errorf("jumpTable \ngot  = %v\nwant = %v", jumpTable, tt.wantJumps)
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
