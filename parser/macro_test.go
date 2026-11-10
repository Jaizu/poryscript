package parser

import (
	"testing"

	"github.com/huderlem/poryscript/ast"
	"github.com/huderlem/poryscript/lexer"
)

func TestMovementMacroStatements(t *testing.T) {
	input := `
movement macro walk_to_gym {
	walk_left * 2
	walk_up
}
movement macro {
	walk_up
}
`
	l := lexer.New(input)
	p := New(l, CommandConfig{}, "", "", 0, nil)
	program, err := p.ParseProgram()
	if err != nil {
		t.Fatalf(err.Error())
	}
	if len(program.TopLevelStatements) != 2 {
		t.Fatalf("Expected 2 top-level statements, got %d", len(program.TopLevelStatements))
	}
	macro := program.TopLevelStatements[0].(*ast.MovementStatement)
	if !macro.IsMacro || macro.Name.Value != "walk_to_gym" || len(macro.MovementCommands) != 3 {
		t.Errorf("Unexpected movement macro: IsMacro=%v Name=%s Commands=%d", macro.IsMacro, macro.Name.Value, len(macro.MovementCommands))
	}
	// A movement that is named "macro" is still a regular movement.
	movement := program.TopLevelStatements[1].(*ast.MovementStatement)
	if movement.IsMacro || movement.Name.Value != "macro" {
		t.Errorf("Expected regular movement named 'macro', got IsMacro=%v Name=%s", movement.IsMacro, movement.Name.Value)
	}
}

func TestTopLevelMacroStatements(t *testing.T) {
	input := `
const LOCALID = 5
const SUM = OFFSET(1) + 2
langtable(MyText)
setup(LOCALID, (1 + 2))
sum(SUM)
noargs()
`
	l := lexer.New(input)
	p := New(l, CommandConfig{}, "", "", 0, nil)
	program, err := p.ParseProgram()
	if err != nil {
		t.Fatalf(err.Error())
	}
	tests := []struct {
		name string
		args []string
	}{
		{"langtable", []string{"MyText"}},
		{"setup", []string{"5", "( 1 + 2 )"}},
		{"sum", []string{"OFFSET ( 1 ) + 2"}},
		{"noargs", []string{}},
	}
	if len(program.TopLevelStatements) != len(tests) {
		t.Fatalf("Expected %d top-level statements, got %d", len(tests), len(program.TopLevelStatements))
	}
	for i, test := range tests {
		command := program.TopLevelStatements[i].(*ast.CommandStatement)
		if command.Name.Value != test.name || len(command.Args) != len(test.args) {
			t.Fatalf("Statement %d: expected %s%v, got %s%v", i, test.name, test.args, command.Name.Value, command.Args)
		}
		for j := range test.args {
			if command.Args[j] != test.args[j] {
				t.Errorf("Statement %d arg %d: expected %q, got %q", i, j, test.args[j], command.Args[j])
			}
		}
	}
}

func TestMacroErrors(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`movement(global) macro walk { walk_up }`, "line 1: movement macros can't have a scope modifier"},
		{`movement macro walk { walk_up }
movement macro walk { face_up }`, "line 2: duplicate movement macro 'walk'"},
		{`langtable("Hello")`, "line 1: top-level macro 'langtable' can't take inline text or movement. Define them with a text or movement statement and pass their labels instead"},
		{`langtable(moves(walk_up))`, "line 1: top-level macro 'langtable' can't take inline text or movement. Define them with a text or movement statement and pass their labels instead"},
		{`langtable(MyText`, "line 1: missing closing parenthesis for command 'langtable'"},
		{`langtable MyText`, "line 1: could not parse top-level statement for 'langtable'"},
	}
	for _, test := range tests {
		l := lexer.New(test.input)
		p := New(l, CommandConfig{}, "", "", 0, nil)
		_, err := p.ParseProgram()
		if err == nil {
			t.Errorf("Expected error %q for input %q, got none", test.expected, test.input)
			continue
		}
		if err.Error() != test.expected {
			t.Errorf("Expected error %q for input %q, got %q", test.expected, test.input, err.Error())
		}
	}
}
