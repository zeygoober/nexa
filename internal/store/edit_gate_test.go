package store

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var accessWrite = regexp.MustCompile(`(?i)(INSERT INTO|DELETE FROM|UPDATE)\s+(roles|channel_overwrites|member_roles)\b|UPDATE\s+channels\s+SET[^;]*parent_id`)

// Builds its SQL from parts, or writes through insertChannel.
var accessWriteByName = []string{"UpdateServer", "UpdateChannel", "CreateServerChannel", "DeleteChannel"}

// What cannot open a channel to anyone an action of the owner's is guarded
// against, or has no server for an action to be under way on.
var accessWriteExempt = map[string]string{
	"CreateServer":             "the server does not exist yet",
	"CreateServerFromTemplate": "the server does not exist yet",
	"PurgeUserLeftovers":       "an account going away only loses things",
	"RemoveMember":             "someone leaving only loses things",
	"SwapRoleIcon":             "a role's picture decides nothing",
	"insertChannel":            "its callers are checked",
	"CreateThread":             "a thread takes its parent's access",
	"createDM":                 "not in a server",
	"CreateGroupDM":            "not in a server",
	"CreateSubchat":            "not in a server",
}

// A new write that can change who sees what in a server has to take the edit
// gate, or a Manage Server action's check could be made against stale facts.
func TestAccessWritesTakeTheEditGate(t *testing.T) {
	pkgs, err := parser.ParseDir(token.NewFileSet(), ".", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	needs, takes := map[string]bool{}, map[string]bool{}
	for _, name := range accessWriteByName {
		needs[name] = true
	}
	seen := map[string]bool{}
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			for _, d := range file.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				name := fn.Name.Name
				seen[name] = true
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					switch n := n.(type) {
					case *ast.BasicLit:
						if n.Kind == token.STRING {
							if v, err := strconv.Unquote(n.Value); err == nil && accessWrite.MatchString(v) {
								needs[name] = true
							}
						}
					case *ast.CallExpr:
						if sel, ok := n.Fun.(*ast.SelectorExpr); ok {
							switch sel.Sel.Name {
							case "ShareServerEdits", "shareChannelEdits", "shareRoleEdits":
								takes[name] = true
							case "insertChannel":
								needs[name] = true
							}
						}
					}
					return true
				})
			}
		}
	}
	for name := range needs {
		if _, exempt := accessWriteExempt[name]; !exempt && !takes[name] {
			t.Errorf("%s writes what decides access but does not take the edit gate", name)
		}
	}
	for name := range accessWriteExempt {
		if !seen[name] {
			t.Errorf("exempt function %s no longer exists", name)
		}
	}
}
