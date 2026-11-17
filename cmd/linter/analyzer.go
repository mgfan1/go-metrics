package main

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"
)

// Analyzer сообщает о вызовах panic, а также log.Fatal и os.Exit вне функции main пакета main.
var Analyzer = &analysis.Analyzer{
	Name: "exitcheck",
	Doc:  "запрещает panic, а также log.Fatal и os.Exit вне функции main пакета main, сгенерированные файлы не проверяет",
	Run:  run,
}

var exitFuncs = map[string]bool{
	"os.Exit":               true,
	"log.Fatal":             true,
	"log.Fatalf":            true,
	"log.Fatalln":           true,
	"(*log.Logger).Fatal":   true,
	"(*log.Logger).Fatalf":  true,
	"(*log.Logger).Fatalln": true,
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if ast.IsGenerated(file) {
			continue
		}

		for _, decl := range file.Decls {
			inMain := isMain(pass, decl)

			ast.Inspect(decl, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					checkCall(pass, call, inMain)
				}
				return true
			})
		}
	}

	return nil, nil
}

func isMain(pass *analysis.Pass, decl ast.Decl) bool {
	fn, ok := decl.(*ast.FuncDecl)
	return ok && fn.Recv == nil && fn.Name.Name == "main" && pass.Pkg.Name() == "main"
}

func checkCall(pass *analysis.Pass, call *ast.CallExpr, inMain bool) {
	switch fn := typeutil.Callee(pass.TypesInfo, call).(type) {
	case *types.Builtin:
		if fn.Name() == "panic" {
			pass.Reportf(call.Pos(), "вызов встроенной функции panic")
		}
	case *types.Func:
		if !inMain && exitFuncs[fn.FullName()] {
			pass.Reportf(call.Pos(), "вызов %s вне функции main пакета main", fn.FullName())
		}
	}
}
