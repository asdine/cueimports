package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
)

const stdinFilename = "<standard input>"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(2)
	}
}

func run() error {
	flag.Parse()

	if flag.NArg() == 0 {
		if err := processInput(stdinFilename, os.Stdin, os.Stdout); err != nil {
			return err
		}
	}

	for i := 0; i < flag.NArg(); i++ {
		path := flag.Arg(i)
		switch dir, err := os.Stat(path); {
		case err != nil:
			return err
		case dir.IsDir():
			if err := filepath.Walk(path, visitFile); err != nil {
				return err
			}
		default:
			if err := processFile(path); err != nil {
				return err
			}
		}
	}
	return nil
}

func visitFile(path string, f os.FileInfo, err error) error {
	if err != nil {
		return err
	}
	if !isCueFile(f) {
		return nil
	}
	return processFile(path)
}

func isCueFile(f os.FileInfo) bool {
	name := f.Name()
	return !f.IsDir() && !strings.HasPrefix(name, ".") && strings.HasSuffix(name, ".cue")
}

func processFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		return err
	}

	res, err := processContent(path, content)
	if err != nil {
		return err
	}

	if bytes.Equal(content, res) {
		return nil
	}

	err = f.Truncate(0)
	if err != nil {
		return err
	}

	_, err = f.Seek(0, 0)
	if err != nil {
		return err
	}

	_, err = f.Write(res)
	return err
}

func processInput(filename string, r io.Reader, w io.Writer) error {
	content, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	res, err := processContent(filename, content)
	if err != nil {
		return err
	}

	_, err = w.Write(res)
	return err
}

func processContent(filename string, content []byte) ([]byte, error) {
	e, err := parser.ParseFile(filename, content)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	unresolved := make(map[string][]string, len(e.Unresolved))

	ast.Walk(e, func(n ast.Node) bool {
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

			for _, u := range e.Unresolved {
				if u.Name == xIdent.Name {
					unresolved[u.Name] = append(unresolved[u.Name], xSel.Name)
				}
			}
		}

		return true
	}, nil)

	// Load other files in the same package and filter out unresolved identifiers
	// that are defined in those files.
	if filename != stdinFilename {
		err = filterSamePackageIdents(unresolved, filename, e.PackageName())
		if err != nil {
			return nil, err
		}
	}

	if len(unresolved) == 0 {
		// nothing to do
		return content, nil
	}

	// resolve imports
	resolved, err := resolve(unresolved, filename)
	if err != nil {
		return nil, err
	}

	// insert resolved imports
	return insertImports(e, resolved)
}

func filterSamePackageIdents(unresolved map[string][]string, filename, packageName string) error {
	dir := filepath.Dir(filename)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() && d.Name() != dir {
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

		ast.Walk(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.Ident:
				if unresolved[x.Name] != nil {
					delete(unresolved, x.Name)
				}
			}

			return true
		}, nil)

		return nil
	})

	return err
}

func resolve(unresolved map[string][]string, filename string) (map[string]string, error) {
	resolved := make(map[string]string)

	// resolve using the stdlib
	resolveInStdlib(unresolved, resolved)

	if len(unresolved) == 0 {
		return resolved, nil
	}

	// resolve using local packages
	err := resolveInLocalPackages(unresolved, resolved, filename)
	if err != nil {
		return nil, err
	}

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
		return fmt.Errorf("could not find cue.mod directory")
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

		// parse the file again, this time parsing the whole file
		f, err = parser.ParseFile(path, content)
		if err != nil {
			return err
		}

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
						resolved[pkgName] = filepath.Join(modName, rel)
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
	parent := from
	for {
		if _, err := os.Stat(filepath.Join(parent, "cue.mod")); err == nil {
			break
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

func insertImports(n *ast.File, resolved map[string]string) ([]byte, error) {
	var modAst ast.Node

	if len(n.Imports) != 0 {
		// insert resolved identifiers as import statements
		modAst = astutil.Apply(n, func(c astutil.Cursor) bool {
			switch x := c.Node().(type) {
			case *ast.ImportDecl:
				for _, r := range resolved {
					x.Specs = append(x.Specs, ast.NewImport(nil, r))
				}
				return false
			}
			return true
		}, nil)
	} else {
		// add import statements
		modAst = astutil.Apply(n, func(c astutil.Cursor) bool {
			switch c.Node().(type) {
			case *ast.Package:
				var idecl ast.ImportDecl

				for _, r := range resolved {
					idecl.Specs = append(idecl.Specs, ast.NewImport(nil, r))
				}
				c.InsertAfter(&idecl)
				return false
			}
			return true
		}, nil)
	}

	return format.Node(modAst)
}
