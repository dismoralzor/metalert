package main

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// marker - маркер в doc-комментарии структуры, включающий генерацию Reset().
const marker = "generate:reset"

// generatedFileName - имя файла, который создаёт генератор в каждом пакете.
const generatedFileName = "reset.gen.go"

// generator хранит множество помеченных типов: оно собирается по ВСЕМ пакетам
// до генерации, чтобы наличие Reset() у типа не зависело от того, успел ли уже
// появиться reset.gen.go (детерминированный вывод).
type generator struct {
	marked map[*types.Named]bool
}

// loadConfigMode - минимальный набор данных для анализа: имена, файлы, AST,
// типы и информация о типах. Тесты не загружаем.
const loadConfigMode = packages.NeedName | packages.NeedFiles |
	packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo

// loadPackages загружает все пакеты модуля из каталога root по паттерну "./...".
// Ошибки типизации (например, от устаревшего reset.gen.go) не валят генератор -
// логируем и продолжаем; падаем только на ошибках загрузки и парсинга.
func loadPackages(root string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Dir:   root,
		Mode:  loadConfigMode,
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("загрузка пакетов: %w", err)
	}

	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			if e.Kind == packages.ParseError || e.Kind == packages.ListError {
				return nil, fmt.Errorf("пакет %s: %s", pkg.PkgPath, e)
			}
			// TypeError - не фатально: структуры в AST всё равно доступны.
			log.Printf("пропускаю ошибку типизации в %s: %s", pkg.PkgPath, e)
		}
	}

	return pkgs, nil
}

// collectMarked собирает множество всех помеченных маркером именованных типов
// по всем пакетам.
func collectMarked(pkgs []*packages.Package) map[*types.Named]bool {
	marked := make(map[*types.Named]bool)

	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			if isGeneratedFile(pkg, file) {
				continue
			}
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, spec := range gen.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if _, ok := ts.Type.(*ast.StructType); !ok {
						continue
					}
					// Комментарий у одиночного `type X struct` висит на GenDecl,
					// у объявления в группе - на самом TypeSpec.
					if !hasMarker(ts.Doc) && !hasMarker(gen.Doc) {
						continue
					}
					if named := namedOf(pkg, ts); named != nil {
						marked[named] = true
					}
				}
			}
		}
	}

	return marked
}

// namedOf возвращает *types.Named для объявления типа ts в пакете pkg.
func namedOf(pkg *packages.Package, ts *ast.TypeSpec) *types.Named {
	obj := pkg.TypesInfo.Defs[ts.Name]
	tn, ok := obj.(*types.TypeName)
	if !ok {
		return nil
	}
	named, _ := tn.Type().(*types.Named)
	return named
}

// hasMarker ищет маркер по СЫРОМУ тексту комментариев: CommentGroup.Text()
// выбрасывает строки вида //generate:reset, считая их директивами.
func hasMarker(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	for _, c := range doc.List {
		text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		if text == marker {
			return true
		}
	}
	return false
}

// isGeneratedFile сообщает, нужно ли пропустить файл: это либо наш reset.gen.go,
// либо любой файл с пометкой "Code generated ... DO NOT EDIT".
func isGeneratedFile(pkg *packages.Package, file *ast.File) bool {
	if ast.IsGenerated(file) {
		return true
	}
	pos := pkg.Fset.Position(file.Pos())
	return filepath.Base(pos.Filename) == generatedFileName
}

// hasReset сообщает, есть ли у типа t метод Reset() без параметров и
// результатов. Помеченный тип считается имеющим Reset всегда (детерминизм).
func (g *generator) hasReset(t types.Type) bool {
	if named, ok := t.(*types.Named); ok && g.marked[named] {
		return true
	}
	// У интерфейса методы на самом типе, у остальных - на указателе
	// (метод Reset объявляется с указательным ресивером).
	if findReset(types.NewMethodSet(t)) {
		return true
	}
	return findReset(types.NewMethodSet(types.NewPointer(t)))
}

// findReset ищет в наборе методов метод Reset() без аргументов и возвращаемых
// значений.
func findReset(ms *types.MethodSet) bool {
	for i := 0; i < ms.Len(); i++ {
		fn, ok := ms.At(i).Obj().(*types.Func)
		if !ok || fn.Name() != "Reset" {
			continue
		}
		sig, ok := fn.Type().(*types.Signature)
		if ok && sig.Params().Len() == 0 && sig.Results().Len() == 0 {
			return true
		}
	}
	return false
}

// emit рекурсивно строит операторы сброса значения expr типа t. Второе
// возвращаемое значение - использован ли generic-хелпер resetZeroGen.
func (g *generator) emit(expr string, t types.Type) ([]string, bool) {
	u := t.Underlying()

	// Поле-значение, тип которого сам умеет Reset (например, вложенная
	// помеченная структура). Указатели и интерфейсы обрабатываем ниже - им
	// нужна защита от nil.
	switch u.(type) {
	case *types.Pointer, *types.Interface:
	default:
		if g.hasReset(t) {
			return []string{expr + ".Reset()"}, false
		}
	}

	switch u := u.(type) {
	case *types.Basic:
		return emitBasic(expr, u)

	case *types.Slice:
		return []string{expr + " = " + expr + "[:0]"}, false

	case *types.Map:
		return []string{"clear(" + expr + ")"}, false

	case *types.Pointer:
		elem := u.Elem()
		if g.hasReset(elem) {
			return wrapNotNil(expr, []string{expr + ".Reset()"}), false
		}
		inner, usesHelper := g.emit("(*"+expr+")", elem)
		return wrapNotNil(expr, inner), usesHelper

	case *types.Interface:
		if g.hasReset(t) {
			return wrapNotNil(expr, []string{expr + ".Reset()"}), false
		}
		return []string{"resetZeroGen(&" + expr + ")"}, true

	default:
		// Структура без Reset, массив, chan, func и т.п.
		return []string{"resetZeroGen(&" + expr + ")"}, true
	}
}

// emitBasic сбрасывает примитив в нулевое значение соответствующего вида.
func emitBasic(expr string, b *types.Basic) ([]string, bool) {
	switch {
	case b.Info()&types.IsString != 0:
		return []string{expr + ` = ""`}, false
	case b.Info()&types.IsBoolean != 0:
		return []string{expr + " = false"}, false
	case b.Info()&types.IsNumeric != 0:
		return []string{expr + " = 0"}, false
	default:
		// unsafe.Pointer и прочая экзотика.
		return []string{"resetZeroGen(&" + expr + ")"}, true
	}
}

// wrapNotNil оборачивает операторы в проверку "if expr != nil { ... }".
func wrapNotNil(expr string, body []string) []string {
	lines := make([]string, 0, len(body)+2)
	lines = append(lines, "if "+expr+" != nil {")
	for _, l := range body {
		lines = append(lines, "\t"+l)
	}
	lines = append(lines, "}")
	return lines
}

// genMethod генерирует метод Reset() для одной помеченной структуры.
func (g *generator) genMethod(named *types.Named) (string, bool) {
	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return "", false
	}

	var b strings.Builder
	name := named.Obj().Name()
	fmt.Fprintf(&b, "func (rs *%s) Reset() {\n", name)
	b.WriteString("\tif rs == nil {\n\t\treturn\n\t}\n")

	usesHelper := false
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Name() == "_" {
			continue
		}
		lines, uh := g.emit("rs."+f.Name(), f.Type())
		usesHelper = usesHelper || uh
		for _, l := range lines {
			b.WriteString("\t" + l + "\n")
		}
	}

	b.WriteString("}\n")
	return b.String(), usesHelper
}

// renderPackage собирает содержимое reset.gen.go для набора помеченных типов
// одного пакета (в стабильном порядке по имени типа).
func (g *generator) renderPackage(pkgName string, named []*types.Named) ([]byte, error) {
	sort.Slice(named, func(i, j int) bool {
		return named[i].Obj().Name() < named[j].Obj().Name()
	})

	var b strings.Builder
	b.WriteString("// Code generated by cmd/reset; DO NOT EDIT.\n\n")
	fmt.Fprintf(&b, "package %s\n\n", pkgName)

	usesHelper := false
	for _, n := range named {
		method, uh := g.genMethod(n)
		usesHelper = usesHelper || uh
		b.WriteString(method)
		b.WriteString("\n")
	}

	// Хелпер добавляем ТОЛЬКО если он реально используется, иначе получится
	// неиспользуемая функция.
	if usesHelper {
		b.WriteString("// resetZeroGen сбрасывает значение в нулевое.\n")
		b.WriteString("func resetZeroGen[T any](p *T) { var zero T; *p = zero }\n")
	}

	return format.Source([]byte(b.String()))
}

// Generate - основная операция: сканирует проект в root, для каждого пакета с
// помеченными структурами пишет reset.gen.go, а из пакетов, где помеченных
// структур не осталось, удаляет устаревший reset.gen.go.
func Generate(root string) error {
	pkgs, err := loadPackages(root)
	if err != nil {
		return err
	}

	g := &generator{marked: collectMarked(pkgs)}

	// Группируем помеченные типы по пакетам (по каталогу пакета).
	byDir := make(map[string][]*types.Named)
	for named := range g.marked {
		dir := packageDir(pkgs, named.Obj().Pkg())
		if dir == "" {
			continue
		}
		byDir[dir] = append(byDir[dir], named)
	}

	for _, pkg := range pkgs {
		dir := dirOf(pkg)
		if dir == "" {
			continue
		}
		genPath := filepath.Join(dir, generatedFileName)

		named := byDir[dir]
		if len(named) == 0 {
			// Помеченных структур нет - убираем устаревший файл, если он был.
			if err := os.Remove(genPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("удаление %s: %w", genPath, err)
			}
			continue
		}

		src, err := g.renderPackage(pkg.Name, named)
		if err != nil {
			return fmt.Errorf("форматирование для %s: %w", pkg.PkgPath, err)
		}
		if err := os.WriteFile(genPath, src, 0o644); err != nil {
			return fmt.Errorf("запись %s: %w", genPath, err)
		}
		log.Printf("сгенерирован %s (%d структур)", genPath, len(named))
	}

	return nil
}

// dirOf возвращает каталог пакета по его исходным файлам.
func dirOf(pkg *packages.Package) string {
	files := pkg.GoFiles
	if len(files) == 0 {
		files = pkg.CompiledGoFiles
	}
	if len(files) == 0 {
		return ""
	}
	return filepath.Dir(files[0])
}

// packageDir находит каталог пакета по его *types.Package.
func packageDir(pkgs []*packages.Package, tp *types.Package) string {
	for _, pkg := range pkgs {
		if pkg.Types == tp {
			return dirOf(pkg)
		}
	}
	return ""
}
