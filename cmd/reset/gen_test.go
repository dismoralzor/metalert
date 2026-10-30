package main

import (
	"go/types"
	"strings"
	"testing"
)

// TestEmit проверяет генерацию операторов сброса для основных видов типов.
func TestEmit(t *testing.T) {
	g := &generator{marked: map[*types.Named]bool{}}

	str := types.Typ[types.String]
	tests := []struct {
		name string
		typ  types.Type
		want string
	}{
		{"string", str, `rs.X = ""`},
		{"bool", types.Typ[types.Bool], "rs.X = false"},
		{"int", types.Typ[types.Int], "rs.X = 0"},
		{"float64", types.Typ[types.Float64], "rs.X = 0"},
		{"slice", types.NewSlice(types.Typ[types.Int]), "rs.X = rs.X[:0]"},
		{"map", types.NewMap(str, str), "clear(rs.X)"},
		{"ptr to string", types.NewPointer(str), "if rs.X != nil {\n\t(*rs.X) = \"\"\n}"},
		{"ptr to int", types.NewPointer(types.Typ[types.Int]), "if rs.X != nil {\n\t(*rs.X) = 0\n}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines, _ := g.emit("rs.X", tt.typ)
			got := strings.Join(lines, "\n")
			if got != tt.want {
				t.Errorf("emit(%s):\n got %q\nwant %q", tt.name, got, tt.want)
			}
		})
	}
}

// TestEmitUsesHelper: структура без Reset и unsafe-подобные типы помечают
// использование хелпера resetZeroGen.
func TestEmitUsesHelper(t *testing.T) {
	g := &generator{marked: map[*types.Named]bool{}}

	// Массив [3]int не имеет Reset и не является примитивом/слайсом/мапой.
	arr := types.NewArray(types.Typ[types.Int], 3)
	lines, usesHelper := g.emit("rs.A", arr)
	if !usesHelper {
		t.Error("для массива ожидалось использование resetZeroGen")
	}
	if got := strings.Join(lines, "\n"); got != "resetZeroGen(&rs.A)" {
		t.Errorf("emit(array) = %q, want resetZeroGen(&rs.A)", got)
	}

	// Примитивы хелпер не используют.
	if _, uh := g.emit("rs.X", types.Typ[types.Int]); uh {
		t.Error("для int хелпер использоваться не должен")
	}
}
