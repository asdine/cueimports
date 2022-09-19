package cueimports

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
)

// Import reads the given cue file and updates the import statements,
// adding missing ones and removing unused ones.
// If content is nil, the file is read from disk,
// otherwise the content is used without reading the file.
// It returns the update file content.
func Import(filename string, content []byte) ([]byte, error) {
	if filename == "" && content == nil {
		return nil, errors.New("filename or content must be provided")
	}

	if filename == "" {
		filename = "_.cue"
	}

	var f *ast.File
	var err error
	opt := []parser.Option{
		parser.ParseComments,
		parser.AllowPartial,
	}
	// ParseFile is too strict and does not allow passing a nil byte slice
	if content == nil {
		f, err = parser.ParseFile(filename, nil, opt...)
	} else {
		f, err = parser.ParseFile(filename, content, opt...)
	}
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	unresolved := make(map[string][]string, len(f.Unresolved))

	// get a list of all unresolved identifiers
	ast.Walk(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			xIdent, ok := x.X.(*ast.Ident)
			if !ok {
				return true
			}
			xSel, ok := x.Sel.(*ast.Ident)
			if !ok {
				return true
			}

			for _, u := range f.Unresolved {
				if u.Name == xIdent.Name {
					unresolved[u.Name] = append(unresolved[u.Name], xSel.Name)
				}
			}
		}

		return true
	}, nil)

	// Load other files in the same package and filter out unresolved identifiers
	// that are defined in those files.
	err = filterSamePackageIdents(unresolved, filename, f.PackageName())
	if err != nil {
		return nil, err
	}

	if len(unresolved) == 0 {
		// nothing to do
		return insertImports(f, nil)
	}

	// resolve imports
	resolved, err := resolve(unresolved, filename)
	if err != nil {
		return nil, err
	}

	// insert resolved imports
	return insertImports(f, resolved)
}

func filterSamePackageIdents(unresolved map[string][]string, filename, packageName string) error {
	dir, err := filepath.Abs(filepath.Dir(filename))
	if err != nil {
		return err
	}
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() && path != dir {
			return filepath.SkipDir
		}

		if !strings.HasSuffix(path, ".cue") {
			return nil
		}

		if d.Name() == filepath.Base(filename) {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(path, content)
		if err != nil {
			return err
		}

		if f.PackageName() != packageName {
			return nil
		}

		for _, decl := range f.Decls {
			switch x := decl.(type) {

			case *ast.Field:
				ident, ok := x.Label.(*ast.Ident)
				if !ok {
					continue
				}
				if unresolved[ident.Name] != nil {
					delete(unresolved, ident.Name)
				}
			}
		}

		return nil
	})

	return err
}

func resolve(unresolved map[string][]string, filename string) (map[string]string, error) {
	resolved := make(map[string]string)

	if len(unresolved) == 0 {
		return resolved, nil
	}

	// resolve using local packages
	err := resolveInLocalPackages(unresolved, resolved, filename)
	if err != nil {
		return nil, err
	}

	// resolve using the stdlib
	resolveInStdlib(unresolved, resolved)

	return resolved, nil
}

// list of std packages as of CUE v0.4.3
var stdPackages = map[string]string{
	"crypto":    "crypto",
	"ed25519":   "crypto/ed25519",
	"hmac":      "crypto/hmac",
	"md5":       "crypto/md5",
	"sha1":      "crypto/sha1",
	"sha256":    "crypto/sha256",
	"sha512":    "crypto/sha512",
	"base64":    "encoding/base64",
	"encoding":  "encoding",
	"csv":       "encoding/csv",
	"hex":       "encoding/hex",
	"json":      "encoding/json",
	"yaml":      "encoding/yaml",
	"html":      "html",
	"list":      "list",
	"math":      "math",
	"bits":      "math/bits",
	"net":       "net",
	"path":      "path",
	"regexp":    "regexp",
	"strconv":   "strconv",
	"strings":   "strings",
	"struct":    "struct",
	"text":      "text",
	"tabwriter": "text/tabwriter",
	"template":  "text/template",
	"time":      "time",
	"tool":      "tool",
	"cli":       "tool/cli",
	"exec":      "tool/exec",
	"file":      "tool/file",
	"http":      "tool/http",
	"os":        "tool/os",
	"uuid":      "uuid",
}

func resolveInStdlib(unresolved map[string][]string, resolved map[string]string) {
	for n := range unresolved {
		if p, ok := stdPackages[n]; ok {
			resolved[n] = p
			delete(unresolved, n)
		}
	}
}

func resolveInLocalPackages(unresolved map[string][]string, resolved map[string]string, filename string) error {
	dir := filepath.Dir(filename)
	// find the cue.mod directory and module name
	modDir, modName, err := findCueModDir(dir)
	if err != nil {
		return fmt.Errorf("find cue.mod: %w", err)
	}

	if modDir == "" {
		// if no cue.mod is found, skip
		return nil
	}

	errStop := errors.New("stop")

	root := filepath.Dir(modDir)
	// resolve from adjacent and sub packages
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			return nil
		}
		if d.Name() == root {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}

		// skip nested cue.mod directories
		if d.Name() == "cue.mod" && path != modDir {
			return filepath.SkipDir
		}

		err = resolveInPackage(unresolved, resolved, path, root, modName)
		if err != nil {
			return err
		}

		if len(unresolved) == 0 {
			return errStop
		}

		return nil
	})
	if err != nil && err != errStop {
		return err
	}

	return nil
}

func resolveInPackage(unresolved map[string][]string, resolved map[string]string, dir, root, modName string) error {
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == dir {
			return nil
		}

		if d.IsDir() && d.Name() != dir {
			return filepath.SkipDir
		}

		if !strings.HasSuffix(path, ".cue") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(path, content, parser.PackageClauseOnly)
		if err != nil {
			return err
		}

		pkgName := f.PackageName()
		s, ok := unresolved[pkgName]
		if !ok {
			// if the package name is not in the unresolved list, we can skip the directory
			return filepath.SkipDir
		}

		// this time parse the whole file
		f, err = parser.ParseFile(path, content)
		if err != nil {
			return err
		}

		// look for one of the identifiers in the file
		ast.Walk(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.Ident:
				for _, name := range s {
					if x.Name == name {
						var rel string
						rel, err = filepath.Rel(root, dir)
						if err != nil {
							return false
						}
						if strings.HasPrefix(rel, "cue.mod/") {
							rel = strings.TrimPrefix(rel, "cue.mod/pkg/")
							rel = strings.TrimPrefix(rel, "cue.mod/usr/")
							resolved[pkgName] = rel
						} else {
							resolved[pkgName] = filepath.Join(modName, rel)
						}
						delete(unresolved, pkgName)
					}
				}
			}

			return true
		}, nil)

		return err
	})

	return err
}

// findCueModDir returns the path of the cue.mod directory.
// find the cue.mod directory
// it can be either in the current directory or in a parent directory
func findCueModDir(from string) (string, string, error) {
	parent, err := filepath.Abs(from)
	if err != nil {
		return "", "", err
	}
LOOP:
	for {
		if _, err := os.Stat(filepath.Join(parent, "cue.mod")); err == nil {
			break LOOP
		}
		if parent == "/" {
			return "", "", nil
		}
		parent = filepath.Dir(parent)
	}

	// get module name
	modDir := filepath.Join(parent, "cue.mod")
	content, err := os.ReadFile(filepath.Join(modDir, "module.cue"))
	if err != nil {
		return "", "", fmt.Errorf("read module.cue: %w", err)
	}
	moduleName := string(bytes.TrimSuffix(bytes.TrimPrefix(content, []byte("module: ")), []byte("\n")))
	module, err := strconv.Unquote(moduleName)
	if err != nil {
		return "", "", fmt.Errorf("unquote module name %s: %w", moduleName, err)
	}

	return modDir, module, nil
}

func insertImports(f *ast.File, resolved map[string]string) ([]byte, error) {
	if resolved == nil {
		resolved = map[string]string{}
	}

	var modAst ast.Node

	var err error
	if len(f.Imports) != 0 {
		// filter out unused imports
		ast.Walk(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.SelectorExpr:
				xx, ok := x.X.(*ast.Ident)
				if !ok {
					return true
				}
				for _, i := range f.Imports {
					var p string
					p, err = strconv.Unquote(i.Path.Value)
					if err != nil {
						return false
					}
					if xx.Name == filepath.Base(p) {
						resolved[xx.Name] = p
					}
				}
			}

			return true
		}, nil)
		if err != nil {
			return nil, err
		}
	}

	// remove all import statements
	modAst = astutil.Apply(f, func(c astutil.Cursor) bool {
		switch c.Node().(type) {
		case *ast.ImportDecl:
			c.Delete()
			return false
		}
		return true
	}, nil)

	if len(resolved) == 0 {
		return format.Node(modAst)
	}

	// sort the imports by group
	// 1. standard library
	// 2. other packages
	// TODO separate local packages from cue.mod/ packages

	var std []string
	for _, p := range resolved {
		// ensure the package actually belongs to the stdlib
		// and not simply ends with the same name
		if fullName, ok := stdPackages[filepath.Base(p)]; ok && fullName == p {
			std = append(std, p)
		}
	}
	sort.Strings(std)

	var local []string
	for _, p := range resolved {
		if fullName, ok := stdPackages[filepath.Base(p)]; !ok || fullName != p {
			local = append(local, p)
		}
	}
	sort.Strings(local)

	var idecl ast.ImportDecl

	for _, r := range std {
		idecl.Specs = append(idecl.Specs, ast.NewImport(nil, r))
	}
	for _, r := range local {
		idecl.Specs = append(idecl.Specs, ast.NewImport(nil, r))
	}

	var inserted bool
	// add single import statements with all resolved imports
	modAst = astutil.Apply(modAst, func(c astutil.Cursor) bool {
		switch c.Node().(type) {
		case *ast.File:
			return true
		case *ast.Package:
			if inserted {
				return false
			}
			inserted = true
			c.InsertAfter(&idecl)
			return false
		default:
			if inserted {
				return false
			}
			inserted = true
			c.InsertBefore(&idecl)
			return false
		}
	}, nil)

	return format.Node(modAst)
}
