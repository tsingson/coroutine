package coroc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Compile compiles coroutines in one or more packages.
//
// The path argument can either be a path to a package, a
// path to a file within a package, or a pattern that matches
// multiple packages (for example, /path/to/package/...).
// The path can be absolute or relative (to the current working
// directory).
func Compile(path string) error {
	if path != "" && !strings.HasSuffix(path, "...") {
		s, err := os.Stat(path)
		if err != nil {
			return err
		} else if !s.IsDir() {
			// Make sure we're loading whole packages.
			path = filepath.Dir(path)
		}
	}
	path = filepath.Clean(path)
	if len(path) > 0 && path[0] != filepath.Separator && path[0] != '.' {
		// Go interprets patterns without a leading dot as part of the
		// stdlib (i.e. part of $GOROOT/src) rather than relative to
		// the working dir. Note that filepath.Join(".", path) does not
		// give the desired result here, hence the manual concat.
		path = "." + string(filepath.Separator) + path
	}

	// Load, parse and type-check packages and their dependencies.
	conf := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedImports | packages.NeedDeps,
	}
	fmt.Println(path)

	pkgs, err := packages.Load(conf, path)
	if err != nil {
		return fmt.Errorf("packages.Load %q: %w", path, err)
	}

	// Make sure the packages are valid. Keep it simple and only return the
	// first err (the goal is to compile coroutines, not worry about error
	// reporting UX).
	for _, p := range pkgs {
		for _, err := range p.Errors {
			return err
		}
	}

	for _, p := range pkgs {
		fmt.Println(p.Name)
	}

	// Generated files must start with: // Code generated by coroc. DO NOT EDIT.

	return nil
}
