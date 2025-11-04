package krtequals

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// TODO(tim): Determine if we need to add a way for devs that define an Equals() method that's
// unrelated to KRT collection to opt out of the analyzer.

// TODO(tim): [krtEqualsNone, krtEqualsTODO] the right naming for these markers?

// Config controls optional checks performed by the krtequals analyzer.
type Config struct {
	// DeepEqual toggles the rule that flags usage of reflect.DeepEqual inside Equals methods.
	// This check is disabled by default for incremental rollout.
	DeepEqual bool `json:"deepEqual"`
	// TODO(time): time.Equals() for direct time.Time comparisons
}

type analyzer struct {
	cfg Config
}

func newAnalyzer(cfg *Config) *analysis.Analyzer {
	a := &analyzer{cfg: *cfg}
	return &analysis.Analyzer{
		Name:     "krtequals",
		Doc:      "Checks Equals() implementations for KRT-style semantic equality issues",
		Run:      a.Run,
		Requires: []*analysis.Analyzer{inspect.Analyzer},
	}
}

type structInfo struct {
	name   string
	fields map[string]*fieldInfo
}

type fieldInfo struct {
	name     string
	pos      token.Pos
	exported bool
	ignore   bool
	todo     bool
}

func (a *analyzer) Run(pass *analysis.Pass) (any, error) {
	ins := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	structs := collectStructs(ins)
	processedEquals := make(map[string]bool)

	ins.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fd, ok := n.(*ast.FuncDecl)
		if !ok || fd.Name == nil || fd.Body == nil {
			return
		}
		if fd.Name.Name != "Equals" {
			return
		}
		if a.cfg.DeepEqual {
			checkReflectDeepEqual(pass, fd)
		}
		if fd.Recv == nil || len(fd.Recv.List) == 0 {
			return
		}

		recvField := fd.Recv.List[0]
		recvTypeName := namedTypeFromExpr(recvField.Type)
		if recvTypeName == "" {
			return
		}
		if processedEquals[recvTypeName] {
			return
		}
		processedEquals[recvTypeName] = true

		recvIdent := ""
		if len(recvField.Names) > 0 && recvField.Names[0] != nil {
			recvIdent = recvField.Names[0].Name
		}

		paramNames := paramIdentsWithType(fd.Type.Params, recvTypeName)

		usedFields := collectUsedFieldsInEquals(fd.Body, recvIdent, paramNames)

		sinfo, ok := structs[recvTypeName]
		if !ok {
			return
		}
		for _, f := range sinfo.fields {
			if !f.exported || f.ignore || f.todo {
				continue
			}
			if !usedFields[f.name] {
				pass.Reportf(f.pos, "field %q in type %q is not used in Equals; either compare it or add // +noKrtEquals", f.name, sinfo.name)
			}
		}
	})

	return nil, nil
}

func collectStructs(ins *inspector.Inspector) map[string]*structInfo {
	structs := make(map[string]*structInfo)

	ins.Preorder([]ast.Node{(*ast.TypeSpec)(nil)}, func(n ast.Node) {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return
		}

		si := &structInfo{
			name:   ts.Name.Name,
			fields: make(map[string]*fieldInfo),
		}

		if st.Fields == nil {
			structs[si.name] = si
			return
		}

		for _, field := range st.Fields.List {
			if len(field.Names) == 0 {
				continue
			}
			ignore, todo := fieldMarkers(field)
			for _, nameIdent := range field.Names {
				if nameIdent == nil {
					continue
				}
				fi := &fieldInfo{
					name:     nameIdent.Name,
					pos:      field.Pos(),
					exported: nameIdent.IsExported(),
					ignore:   ignore,
					todo:     todo,
				}
				si.fields[fi.name] = fi
			}
		}

		structs[si.name] = si
	})

	return structs
}

func checkReflectDeepEqual(pass *analysis.Pass, fd *ast.FuncDecl) {
	if pass.TypesInfo == nil {
		return
	}

	ast.Inspect(fd.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}

		obj := pass.TypesInfo.Uses[pkgIdent]
		pkgName, ok := obj.(*types.PkgName)
		if !ok {
			return true
		}

		if pkgName.Imported().Path() == "reflect" && sel.Sel.Name == "DeepEqual" {
			pass.Reportf(call.Pos(), "Equals() method uses reflect.DeepEqual which is slow and ignores //+noKrtEquals markers")
		}

		return true
	})
}

func namedTypeFromExpr(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

func paramIdentsWithType(params *ast.FieldList, typeName string) []string {
	if params == nil {
		return nil
	}
	var out []string
	for _, field := range params.List {
		if !sameNamedType(field.Type, typeName) {
			continue
		}
		for _, nameIdent := range field.Names {
			if nameIdent != nil && nameIdent.Name != "" {
				out = append(out, nameIdent.Name)
			}
		}
	}
	return out
}

func sameNamedType(expr ast.Expr, typeName string) bool {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name == typeName
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name == typeName
		}
	}
	return false
}

func collectUsedFieldsInEquals(body *ast.BlockStmt, recvIdent string, paramIdents []string) map[string]bool {
	used := make(map[string]bool)
	if body == nil {
		return used
	}

	paramSet := make(map[string]struct{}, len(paramIdents))
	for _, p := range paramIdents {
		paramSet[p] = struct{}{}
	}

	ast.Inspect(body, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}

		if id.Name == recvIdent {
			used[sel.Sel.Name] = true
			return true
		}

		if _, ok := paramSet[id.Name]; ok {
			used[sel.Sel.Name] = true
		}

		return true
	})

	return used
}

func fieldMarkers(field *ast.Field) (ignore bool, todo bool) {
	ignoreDoc, todoDoc := extractSpecialMarkers(field.Doc)
	ignoreLine, todoLine := extractSpecialMarkers(field.Comment)
	return ignoreDoc || ignoreLine, todoDoc || todoLine
}

func extractSpecialMarkers(cg *ast.CommentGroup) (ignore bool, todo bool) {
	if cg == nil {
		return
	}
	for _, c := range cg.List {
		text := strings.ToLower(c.Text)
		if strings.Contains(text, "+krtequalstodo") {
			ignore = true
			todo = true
		}
		if strings.Contains(text, "+nokrtequals") {
			ignore = true
		}
	}
	return
}
