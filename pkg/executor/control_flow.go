package executor

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Keeps track of where the Program Counter (PC) should move next.
// PC is just the index of the slice of instructions.
// On true, we always execute the next immediate line.
type Jump struct {
	TargetOnFalse int // If -> elif; elif -> else; while -> endwhile
	TargetOnEnd   int // If/elif/else -> endif; endwhile -> while
}

// We strip out comments and blanks, so track original line# for debugging
type Instruction struct {
	Text   string
	LineNo int
	Type   string
}

type stackframe struct {
	typ string
	pc  int
}

type breakPoint struct {
	loopLvl int
	pc      int
}

func parseJumpTable(r io.Reader) ([]Instruction, map[int]Jump, error) {
	// Results placeholder
	var instructions []Instruction
	var jumpTable = make(map[int]Jump)

	// Variables for processing batch
	pc, lineNo := 0, 0
	var stack []stackframe

	// Breaks requiring cutting across if blocks, easier to track it seperately
	// On reaching endwhile we will only set those breaks encountered within scope
	var loopLvl int
	var breakpoints []breakPoint

	for scanner := bufio.NewScanner(r); scanner.Scan(); {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}

		var instr Instruction
		switch first, rest, _ := strings.Cut(line, " "); first {
		case "if", "elif", "while":
			instr = Instruction{Text: rest, LineNo: lineNo, Type: first}
		case "else", "endif", "endwhile", "break", "exit":
			// Syntax purity - single worded keywords
			if rest != "" {
				err := fmt.Errorf("syntax error: unexpected token following '%s' on line %d", first, lineNo)
				return nil, nil, err
			}
			instr = Instruction{Text: rest, LineNo: lineNo, Type: first}
		default:
			instr = Instruction{Text: line, LineNo: lineNo}
		}

		// Prev stackframe if available
		var prev stackframe
		if len(stack) > 0 {
			prev = stack[len(stack)-1]
		}

		switch instr.Type {
		case "exit":
			// Do nothing, handled by EvalBatch

		case "if", "while":
			stack = append(stack, stackframe{typ: instr.Type, pc: pc})
			if instr.Type == "while" {
				loopLvl++
			}

		case "elif", "else":
			if prev.typ != "if" && prev.typ != "elif" {
				err := fmt.Errorf("syntax error: '%s' block without a preceding if/elif block at line %d", instr.Type, lineNo)
				return nil, nil, err
			}

			// Prev stack entry on false should jump to here
			jumpTable[prev.pc] = Jump{TargetOnFalse: pc}

			// We still need to set 'TargetOnEnd' for prev,
			// for now just add current frame into stack
			stack = append(stack, stackframe{typ: instr.Type, pc: pc})

		case "endif":
			if prev.typ != "if" && prev.typ != "elif" && prev.typ != "else" {
				err := fmt.Errorf("syntax error: 'endif' without a preceding if/elif/else block at line %d", lineNo)
				return nil, nil, err
			}

			// Continue popping until first if block, setting 'TargetOnEnd' for all
			// Guaranteed, we will not have a while/break/endwhile in between from above checks
			for len(stack) > 0 {
				prevIf := stack[len(stack)-1]
				stack = stack[:len(stack)-1]

				// We won't have an jumpTable entry for most stackframe
				// For elif...endif, we set elif.TargetOnFalse = endif
				jump, ok := jumpTable[prevIf.pc]
				if !ok && prevIf.typ != "else" {
					jump.TargetOnFalse = pc
				}
				jump.TargetOnEnd = pc
				jumpTable[prevIf.pc] = jump

				// First if block reached (if .. elif .. else .. endif)
				if prevIf.typ == "if" {
					break
				}
			}

		case "endwhile":
			if prev.typ != "while" {
				err := fmt.Errorf("syntax error: 'endwhile' without a preceding while block at line %d", lineNo)
				return nil, nil, err
			}

			stack = stack[:len(stack)-1]

			// while.TargetOnFalse = endwhile
			jump := jumpTable[prev.pc]
			jump.TargetOnFalse = pc
			jumpTable[prev.pc] = jump

			// endwhile.TargetOnEnd = while
			jumpTable[pc] = Jump{TargetOnEnd: prev.pc}

			// Set all break's pc to this endwhile
			for len(breakpoints) > 0 {
				size := len(breakpoints)
				if bp := breakpoints[size-1]; bp.loopLvl < loopLvl {
					// break in outer while (while ... BREAK .. while .. endwhile)
					break
				} else {
					// bp.loopLvl can never be > loopLvl logically, only ==
					jumpTable[bp.pc] = Jump{TargetOnEnd: pc}
					breakpoints = breakpoints[:size-1]
				}
			}

			// Decrement post assigning bkpts TargetOnEnd
			loopLvl--

		case "break":
			if loopLvl <= 0 {
				return nil, nil, fmt.Errorf("syntax error: 'break' outside of a looping construct at line %d", lineNo)
			}
			breakpoints = append(breakpoints, breakPoint{loopLvl: loopLvl, pc: pc})
		}

		instructions = append(instructions, instr)
		pc++
	}

	if len(stack) > 0 {
		last := stack[len(stack)-1]
		return nil, nil, fmt.Errorf("syntax error: unclosed '%s' block starting at line# %d",
			last.typ, instructions[last.pc].LineNo)
	}

	return instructions, jumpTable, nil
}
