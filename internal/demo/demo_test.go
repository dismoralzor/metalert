package demo

import "testing"

func TestDemo_Reset(t *testing.T) {
	note := "hello"
	d := &Demo{
		Count:   7,
		Name:    "server",
		Enabled: true,
		Note:    &note,
		Numbers: make([]int, 0, 8),
		Labels:  map[string]string{"a": "b", "c": "d"},
		Child:   &Demo{Count: 99, Name: "child", Enabled: true},
		Extra:   nestedNote{Code: 5, Text: "note"},
	}
	d.Numbers = append(d.Numbers, 1, 2, 3)
	capBefore := cap(d.Numbers)

	d.Reset()

	// Примитивы - нулевые.
	if d.Count != 0 || d.Name != "" || d.Enabled {
		t.Errorf("примитивы не сброшены: %+v", d)
	}
	// *string: указатель сохранён и указывает на "".
	if d.Note == nil {
		t.Fatal("Note стал nil, ожидался указатель на \"\"")
	}
	if *d.Note != "" {
		t.Errorf("*Note = %q, want \"\"", *d.Note)
	}
	// Слайс: длина 0, ёмкость сохранена.
	if len(d.Numbers) != 0 {
		t.Errorf("len(Numbers) = %d, want 0", len(d.Numbers))
	}
	if cap(d.Numbers) != capBefore {
		t.Errorf("cap(Numbers) = %d, want %d (ёмкость должна сохраниться)", cap(d.Numbers), capBefore)
	}
	// Мапа: пустая, но не nil.
	if d.Labels == nil {
		t.Fatal("Labels стал nil, ожидалась пустая не-nil мапа")
	}
	if len(d.Labels) != 0 {
		t.Errorf("len(Labels) = %d, want 0", len(d.Labels))
	}
	// Самоссылка: сброшена рекурсивно (указатель сохранён, поля обнулены).
	if d.Child == nil {
		t.Fatal("Child стал nil, ожидался рекурсивный сброс")
	}
	if d.Child.Count != 0 || d.Child.Name != "" || d.Child.Enabled {
		t.Errorf("Child не сброшен рекурсивно: %+v", d.Child)
	}
	// Вложенная структура без Reset - обнулена целиком.
	if d.Extra.Code != 0 || d.Extra.Text != "" {
		t.Errorf("Extra не обнулён: %+v", d.Extra)
	}
}

func TestDemo_Reset_NilReceiver(t *testing.T) {
	var d *Demo
	// Не должно паниковать.
	d.Reset()
}
