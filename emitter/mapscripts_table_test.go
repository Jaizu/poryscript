package emitter

import (
	"testing"

	"github.com/huderlem/poryscript/lexer"
	"github.com/huderlem/poryscript/parser"
)

func TestEmitMapScriptsTable(t *testing.T) {
	input := `
const STATE = VAR_TEMP_1
mapscripts Shared_OnFrame [
    STATE, 0: Shared_EventScript_Start
    STATE, 1 {
        setvar(STATE, 2)
    }
]

mapscripts(local) Local_OnWarp [
    VAR_TEMP_0, 0: Shared_EventScript_Start
]

mapscripts Map_MapScripts {
    MAP_SCRIPT_ON_FRAME_TABLE: Shared_OnFrame
    MAP_SCRIPT_ON_WARP_INTO_MAP_TABLE [
        VAR_TEMP_0, 0: Shared_EventScript_Start
    ]
}

script Shared_EventScript_Start {
    end
}
`
	expected := `Shared_OnFrame::
	map_script_2 VAR_TEMP_1, 0, Shared_EventScript_Start
	map_script_2 VAR_TEMP_1, 1, Shared_OnFrame_1
	.2byte 0

Shared_OnFrame_1:
	setvar VAR_TEMP_1, 2
	return


Local_OnWarp:
	map_script_2 VAR_TEMP_0, 0, Shared_EventScript_Start
	.2byte 0


Map_MapScripts::
	map_script MAP_SCRIPT_ON_FRAME_TABLE, Shared_OnFrame
	map_script MAP_SCRIPT_ON_WARP_INTO_MAP_TABLE, Map_MapScripts_MAP_SCRIPT_ON_WARP_INTO_MAP_TABLE
	.byte 0

Map_MapScripts_MAP_SCRIPT_ON_WARP_INTO_MAP_TABLE:
	map_script_2 VAR_TEMP_0, 0, Shared_EventScript_Start
	.2byte 0


Shared_EventScript_Start::
	end

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
}

func TestMapScriptsTableErrors(t *testing.T) {
	tests := []struct {
		input string
		err   string
	}{
		{`mapscripts T [ VAR_TEMP_0 0: S ]`, "line 1: missing ',' to specify map script table entry comparison value"},
		{`mapscripts T [ VAR_TEMP_0, 0 S ]`, "line 1: missing ':' or '{' to specify map script table entry"},
	}
	for _, test := range tests {
		l := lexer.New(test.input)
		p := parser.New(l, parser.CommandConfig{}, "", "", 0, nil)
		_, err := p.ParseProgram()
		if err == nil || err.Error() != test.err {
			t.Errorf("input %q: expected error %q, got %v", test.input, test.err, err)
		}
	}
}
