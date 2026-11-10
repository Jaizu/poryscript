package emitter

import (
	"testing"

	"github.com/huderlem/poryscript/lexer"
	"github.com/huderlem/poryscript/parser"
)

func TestEmitMovementMacrosAndTopLevelMacros(t *testing.T) {
	input := `
const COUNT = 3
movement PlayerWalk {
	walk_down
	walk_to_gym * 2
}

movement macro walk_to_gym {
	walk_left * 3
	walk_up
}

text Greeting_En { "Hi$" }
langtable(Greeting)
mymacro(1, COUNT, (2 + 3))
noargs()

script Foo {
	applymovement(1, moves(walk_to_gym face_up))
}
`
	expected := `.macro walk_to_gym
	walk_left
	walk_left
	walk_left
	walk_up
.endm

PlayerWalk:
	walk_down
	walk_to_gym
	walk_to_gym
	step_end

	langtable Greeting

	mymacro 1, 3, ( 2 + 3 )

	noargs

Foo::
	applymovement 1, Foo_Movement_0
	return


Foo_Movement_0:
	walk_to_gym
	face_up
	step_end

Greeting_En::
	.string "Hi$"
`
	expectedLineMarkers := `# 8 "test.pory"
.macro walk_to_gym
# 9 "test.pory"
	walk_left
# 9 "test.pory"
	walk_left
# 9 "test.pory"
	walk_left
# 10 "test.pory"
	walk_up
.endm
`
	l := lexer.New(input)
	p := parser.New(l, parser.CommandConfig{}, "", "", 0, nil)
	program, err := p.ParseProgram()
	if err != nil {
		t.Fatalf(err.Error())
	}
	result, err := New(program, true, false, "").Emit()
	if err != nil {
		t.Fatalf(err.Error())
	}
	if result != expected {
		t.Errorf("Mismatching emit -- Expected=%q, Got=%q", expected, result)
	}

	result, err = New(program, true, true, "test.pory").Emit()
	if err != nil {
		t.Fatalf(err.Error())
	}
	if len(result) < len(expectedLineMarkers) || result[:len(expectedLineMarkers)] != expectedLineMarkers {
		t.Errorf("Mismatching line markers -- Expected prefix=%q, Got=%q", expectedLineMarkers, result)
	}
}
