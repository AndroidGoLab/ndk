// Command ndkcli-commands parses cmd/ndkcli/ source files and prints
// the full cobra command tree without compiling the binary (which
// requires Android NDK libraries).
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
)

type cmdInfo struct {
	varName string
	use     string
	short   string
}

func main() {
	dir := "cmd/ndkcli"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	fset := token.NewFileSet()
	// Parse every target's source so the report covers the complete generated CLI.
	//nolint:staticcheck // Build-aware package loading would omit cross-target commands.
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		os.Exit(1)
	}

	// Collect all cobra.Command variable declarations.
	commands := map[string]cmdInfo{}
	// Collect AddCommand calls to build parent→child relationships.
	children := map[string][]string{} // parent var → child vars

	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			collectCommands(file, commands)
			collectAddCommands(file, children)
		}
	}

	// Build tree from rootCmd down.
	printTree("rootCmd", commands, children, "ndkcli", 0)
}

func collectCommands(
	file *ast.File,
	commands map[string]cmdInfo,
) {
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			continue
		}
		for _, spec := range genDecl.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
				continue
			}
			varName := vs.Names[0].Name

			// Look for &cobra.Command{...} composite literals.
			unary, ok := vs.Values[0].(*ast.UnaryExpr)
			if !ok {
				continue
			}
			comp, ok := unary.X.(*ast.CompositeLit)
			if !ok {
				continue
			}

			use, short := extractFields(comp)
			if use == "" {
				continue
			}
			commands[varName] = cmdInfo{
				varName: varName,
				use:     use,
				short:   short,
			}
		}
	}
}

func extractFields(
	comp *ast.CompositeLit,
) (string, string) {
	var use, short string
	for _, elt := range comp.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		val, ok := kv.Value.(*ast.BasicLit)
		if !ok || val.Kind != token.STRING {
			continue
		}
		v := strings.Trim(val.Value, "\"")
		switch key.Name {
		case "Use":
			use = v
		case "Short":
			short = v
		}
	}
	return use, short
}

func collectAddCommands(
	file *ast.File,
	children map[string][]string,
) {
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "AddCommand" {
			return true
		}
		parentIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		parent := parentIdent.Name
		for _, arg := range call.Args {
			childIdent, ok := arg.(*ast.Ident)
			if !ok {
				continue
			}
			children[parent] = append(children[parent], childIdent.Name)
		}
		return true
	})
}

func printTree(
	varName string,
	commands map[string]cmdInfo,
	children map[string][]string,
	prefix string,
	depth int,
) {
	kids := children[varName]
	sort.Slice(kids, func(i, j int) bool {
		return commands[kids[i]].use < commands[kids[j]].use
	})

	for _, child := range kids {
		info, ok := commands[child]
		if !ok {
			continue
		}
		use := info.use
		fullPath := prefix + " " + use

		subKids := children[child]
		if len(subKids) > 0 {
			// This is a group command — recurse.
			printTree(child, commands, children, fullPath, depth+1)
		} else {
			// Leaf command.
			fmt.Printf("%-50s  %s\n", fullPath, info.short)
		}
	}
}
