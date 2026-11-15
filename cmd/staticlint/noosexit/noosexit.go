// Package noosexit содержит анализатор, запрещающий прямой вызов os.Exit
// в функции main пакета main.
package noosexit

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// Analyzer запрещает прямой вызов os.Exit в функции main пакета main.
// Прямой выход из main минует graceful shutdown и defer'ы, поэтому логику
// завершения выносят в отдельную функцию, а в main оставляют log.Fatal.
var Analyzer = &analysis.Analyzer{
	Name: "noosexit",
	Doc:  "запрещает прямой вызов os.Exit в функции main пакета main",
	Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	// Анализатор касается только пакета main - в библиотечных пакетах функция
	// main не имеет особого смысла и под запрет не попадает.
	if pass.Pkg.Name() != "main" {
		return nil, nil
	}

	for _, file := range pass.Files {
		// Сгенерированные файлы пропускаем: при тестах Go генерирует _testmain.go
		// с os.Exit(m.Run()) в main - это не пользовательский код.
		if ast.IsGenerated(file) {
			continue
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			// Именно функция main (Recv == nil отсекает метод с именем main).
			if !ok || fn.Recv != nil || fn.Name.Name != "main" {
				continue
			}
			checkFuncBody(pass, fn.Body)
		}
	}

	return nil, nil
}

// checkFuncBody обходит тело функции и репортит каждый прямой вызов os.Exit.
func checkFuncBody(pass *analysis.Pass, body *ast.BlockStmt) {
	if body == nil {
		return
	}

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if isOSExit(pass, call) {
			pass.Reportf(call.Pos(), "прямой вызов os.Exit в функции main пакета main")
		}
		return true
	})
}

// isOSExit определяет вызов os.Exit по типам, а не по тексту: так распознаётся
// и импорт пакета os под алиасом.
func isOSExit(pass *analysis.Pass, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	obj, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || obj.Pkg() == nil {
		return false
	}

	return obj.Pkg().Path() == "os" && obj.Name() == "Exit"
}
