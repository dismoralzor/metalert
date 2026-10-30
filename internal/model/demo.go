package models

// nestedNote - вложенная структура-данные без собственного Reset(): генератор
// сбросит её целиком через resetZeroGen.
type nestedNote struct {
	Code int
	Text string
}

// Demo - демонстрационная структура для кодогенератора cmd/reset: показывает
// сброс полей разных видов (примитивы, указатель, слайс, мапа, самоссылка и
// вложенная структура). Маркер ниже включает генерацию метода Reset().
//
// generate:reset
type Demo struct {
	Count   int
	Name    string
	Enabled bool
	Note    *string
	Numbers []int
	Labels  map[string]string
	Child   *Demo // самоссылка - сбрасывается рекурсивно через Reset()
	Extra   nestedNote
}
