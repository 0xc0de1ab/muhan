package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

var fieldToDomain = map[string]string{
	"players": "playersMu", "spies": "playersMu",
	"rooms": "roomsMu", "trapStates": "roomsMu",
	"creatures": "creaturesMu", "effectExpirations": "creaturesMu", "monsterDamage": "creaturesMu", "cooldowns": "creaturesMu", "enemies": "creaturesMu", "lightTimers": "creaturesMu",
	"objects": "objectsMu", "prototypes": "objectsMu",
	"banks": "banksMu",
	"families": "familiesMu", "marriageInvites": "familiesMu", "familyWar": "familiesMu",
	"lockouts": "miscMu", "dbRoot": "miscMu", "shutdownLTime": "miscMu", "shutdownInterval": "miscMu", "lastShutdownUpdate": "miscMu", "lastActiveUpdate": "miscMu", "lastPlayerUpdate": "miscMu", "lastRandomUpdate": "miscMu", "lastTimeUpdate": "miscMu", "lastExitUpdate": "miscMu", "legacyTime": "miscMu", "randomUpdateInterval": "miscMu", "txInterval": "miscMu", "saveQueue": "miscMu",
}

var allMu = []string{"miscMu", "familiesMu", "banksMu", "objectsMu", "creaturesMu", "roomsMu", "playersMu"}

func main() {
	fset := token.NewFileSet()
	files, _ := filepath.Glob("internal/world/state/*.go")
	
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") { continue }
		f, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
		if err != nil { panic(err) }
		
		modified := false
		
		ast.Inspect(f, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok { return true }
			
			domainsUsed := make(map[string]bool)
			
			ast.Inspect(fn, func(nn ast.Node) bool {
				if sel, ok := nn.(*ast.SelectorExpr); ok {
					if id, ok := sel.X.(*ast.Ident); ok && id.Name == "w" {
						if dom, exists := fieldToDomain[sel.Sel.Name]; exists {
							domainsUsed[dom] = true
						}
					}
				}
				return true
			})
			
			// Replace w.mu.Lock() and Unlock()
			ast.Inspect(fn, func(nn ast.Node) bool {
				if call, ok := nn.(*ast.CallExpr); ok {
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
						if isWMu(sel.X) {
							if sel.Sel.Name == "Lock" || sel.Sel.Name == "Unlock" || sel.Sel.Name == "RLock" || sel.Sel.Name == "RUnlock" {
								// Found a call! We need to replace it in the file.
                                // It's easier to just do string replacement using exact positions!
							}
						}
					}
				}
				return true
			})
			
			return true
		})
	}
}

func isWMu(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok { return false }
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == "w" && sel.Sel.Name == "mu"
}
