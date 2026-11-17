package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

var update = flag.Bool("update", false, "перезаписать эталоны в testdata")

const goMod = "module example.com/sample\n\ngo 1.24\n"

func TestRun(t *testing.T) {
	dir := copyModule(t, "testdata/sample")

	for range 2 {
		require.NoError(t, run(dir))

		checkGolden(t, filepath.Join(dir, outputName), "testdata/sample.golden")
		checkGolden(t, filepath.Join(dir, "inner", outputName), "testdata/inner.golden")
		assert.NoFileExists(t, filepath.Join(dir, "plain", outputName))
	}

	checkCompiles(t, dir)
}

func TestRunFromPackageDir(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"lib/lib.go": "package lib\n\n// generate:reset\ntype L struct {\n\tItems []int\n}\n",
		"app/app.go": "package app\n\nimport \"example.com/sample/lib\"\n\n// generate:reset\ntype App struct {\n\tL lib.L\n}\n",
	})

	require.NoError(t, run(dir))
	fromRoot, err := os.ReadFile(filepath.Join(dir, "app", outputName))
	require.NoError(t, err)

	require.NoError(t, run(filepath.Join(dir, "app")))
	fromPackage, err := os.ReadFile(filepath.Join(dir, "app", outputName))
	require.NoError(t, err)

	assert.Contains(t, string(fromRoot), "a.L.Reset()")
	assert.Equal(t, string(fromRoot), string(fromPackage))
}

func TestRunRewritesStaleOutput(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"a/a.go":         "package a\n\n// generate:reset\ntype A struct {\n\tN int\n}\n",
		"a/reset.gen.go": header + "\n\npackage a\n\nfunc (a *A) Reset() {\n\ta.Old = nil\n}\n",
		"b/b.go":         "package b\n\ntype B struct {\n\tN int\n}\n",
		"b/reset.gen.go": header + "\n\npackage b\n\nfunc (b *B) Reset() {\n\tb.N = 0\n}\n",
		"c/c.go":         "package c\n\ntype C struct {\n\tN int\n}\n",
		"c/reset.gen.go": "package c\n\nconst Version = 1\n",
	})

	require.NoError(t, run(dir))

	src, err := os.ReadFile(filepath.Join(dir, "a", outputName))
	require.NoError(t, err)
	assert.Contains(t, string(src), "a.N = 0")
	assert.NotContains(t, string(src), "Old")
	assert.NoFileExists(t, filepath.Join(dir, "b", outputName))
	assert.FileExists(t, filepath.Join(dir, "c", outputName))
	checkCompiles(t, dir)
}

func TestRunWhenResetIsAlreadyCalled(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"a/a.go": "package a\n\n// generate:reset\ntype A struct {\n\tN int\n}\n\nfunc Release(a *A) {\n\ta.Reset()\n}\n",
	})

	require.NoError(t, run(dir))

	assert.FileExists(t, filepath.Join(dir, "a", outputName))
	checkCompiles(t, dir)
}

func TestRunWithoutPackages(t *testing.T) {
	err := run(t.TempDir())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "не найдено пакетов Go")
}

func TestRunErrors(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name:  "не структура",
			files: map[string]string{"a/a.go": "package a\n\n// generate:reset\ntype ID int\n"},
			want:  "ID не структура",
		},
		{
			name:  "свой Reset",
			files: map[string]string{"a/a.go": "package a\n\n// generate:reset\ntype A struct{ N int }\n\nfunc (a *A) Reset() { a.N = 1 }\n"},
			want:  "уже есть свой метод Reset",
		},
		{
			name:  "обобщённый тип",
			files: map[string]string{"a/a.go": "package a\n\n// generate:reset\ntype Box[T any] struct{ V T }\n"},
			want:  "обобщённые типы не поддерживаются",
		},
		{
			name: "чужой reset.gen.go",
			files: map[string]string{
				"a/a.go":         "package a\n\n// generate:reset\ntype A struct{ N int }\n",
				"a/reset.gen.go": "package a\n\nconst Version = 1\n",
			},
			want: "создан не генератором",
		},
		{
			name:  "неизвестный тип поля",
			files: map[string]string{"a/a.go": "package a\n\n// generate:reset\ntype A struct {\n\tM Missing\n}\n"},
			want:  "поле M",
		},
		{
			name:  "синтаксическая ошибка",
			files: map[string]string{"a/a.go": "package a\n\nfunc broken( {\n"},
			want:  "expected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeModule(t, tt.files)

			err := run(dir)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func copyModule(t *testing.T, src string) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.CopyFS(dir, os.DirFS(src)))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644))

	return dir
}

func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644))

	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	return dir
}

func checkGolden(t *testing.T, path, golden string) {
	t.Helper()

	got, err := os.ReadFile(path)
	require.NoError(t, err)

	if *update {
		require.NoError(t, os.WriteFile(golden, got, 0o644))
	}

	want, err := os.ReadFile(golden)
	require.NoError(t, err)
	assert.Equal(t, strings.ReplaceAll(string(want), "\r\n", "\n"), string(got))
}

func checkCompiles(t *testing.T, dir string) {
	t.Helper()

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedImports | packages.NeedDeps | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  dir,
	}
	pkgs, err := packages.Load(cfg, "./...")
	require.NoError(t, err)

	for _, pkg := range pkgs {
		assert.Empty(t, pkg.Errors, pkg.PkgPath)
	}
}
