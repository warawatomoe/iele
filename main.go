package main

import (
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	e "iele/internal/err"
)

//go:embed internal
var internalFS embed.FS

//go:embed templates
var templateFS embed.FS

var mandatory = []string{"arg", "err", "pipe"}
var optional = map[string]bool{
	"cfg": true, "proc": true, "sec": true, "tmp": true, "turn": true, "wal": true, "web": true,
}
var licNames = map[string]string{
	"mit": "MIT", "bsd-2": "BSD-2", "apache-2": "APACHE-2",
}

var version = "dev"

func main() {
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	versionFlag := fs.Bool("v", false, "print version")
	name := fs.String("n", "", "")
	author := fs.String("a", "", "")
	license := fs.String("l", "mit", "")
	pkgs := fs.String("p", "", "")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: iele -n name -a author [-l license] [-p packages] [-v]\n")
		fmt.Fprintf(os.Stderr, "  -n  project name\n")
		fmt.Fprintf(os.Stderr, "  -a  author\n")
		fmt.Fprintf(os.Stderr, "  -l  license: mit, bsd-2, apache-2 (default: mit)\n")
		fmt.Fprintf(os.Stderr, "  -p  optional packages: cfg,proc,sec,tmp,turn,wal,web\n")
		fmt.Fprintf(os.Stderr, "  -v  print version\n")
	}

	if err := fs.Parse(os.Args[1:]); err != nil {
		e.Die(e.New("iele", e.Call, "main", err.Error()))
	}

	if *versionFlag {
		fmt.Printf("iele %s\n", version)
		os.Exit(0)
	}

	if *name == "" || *author == "" {
		fs.Usage()
		e.Die(e.New("iele", e.Call, "main", "name_and_author_required"))
	}

	licFile, ok := licNames[*license]
	if !ok {
		e.Die(e.New("iele", e.Call, "main", "invalid_license"))
	}

	selected := make(map[string]bool)
	for _, m := range mandatory {
		selected[m] = true
	}
	if *pkgs != "" {
		for _, p := range strings.Split(*pkgs, ",") {
			p = strings.TrimSpace(p)
			if !optional[p] {
				e.Die(e.New("iele", e.Call, "main", "unknown_pkg_"+p))
			}
			selected[p] = true
		}
	}

	if err := os.MkdirAll(*name, 0755); err != nil {
		e.Die(e.Wrap("iele", e.Trans, "mkdir", err))
	}

	for pkg := range selected {
		copyPkg(*name, pkg)
	}

	stampLicense(*name, licFile, *author)
	stampMain(*name, *author)

	gomod := fmt.Sprintf("module %s\n\ngo 1.22\n", *name)
	if err := os.WriteFile(filepath.Join(*name, "go.mod"), []byte(gomod), 0644); err != nil {
		e.Die(e.Wrap("iele", e.Trans, "gomod", err))
	}

	fmt.Printf("stamped %s\n", *name)
}

func copyPkg(name, pkg string) {
	srcDir := path.Join("internal", pkg)
	err := fs.WalkDir(internalFS, srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dst := filepath.Join(name, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(dst, 0755)
		}
		data, err := internalFS.ReadFile(p)
		if err != nil {
			return err
		}
		content := strings.ReplaceAll(string(data), "\"iele/internal", "\""+name+"/internal")
		return os.WriteFile(dst, []byte(content), 0644)
	})
	if err != nil {
		e.Die(e.Wrap("iele", e.Trans, "copy_"+pkg, err))
	}
}

func stampLicense(name, licFile, author string) {
	data, err := templateFS.ReadFile(path.Join("templates", "licenses", licFile))
	if err != nil {
		e.Die(e.Wrap("iele", e.Trans, "lic_read", err))
	}
	tmpl, err := template.New("lic").Parse(string(data))
	if err != nil {
		e.Die(e.Wrap("iele", e.Bug, "lic_parse", err))
	}
	f, err := os.Create(filepath.Join(name, "LICENSE"))
	if err != nil {
		e.Die(e.Wrap("iele", e.Trans, "lic_create", err))
	}
	defer f.Close()
	if err := tmpl.Execute(f, map[string]string{
		"Year":   fmt.Sprintf("%d", time.Now().Year()),
		"Author": author,
	}); err != nil {
		e.Die(e.Wrap("iele", e.Trans, "lic_execute", err))
	}
}

func stampMain(name, author string) {
	data, err := templateFS.ReadFile("templates/main.go.tpl")
	if err != nil {
		e.Die(e.Wrap("iele", e.Trans, "main_read", err))
	}
	tmpl, err := template.New("main").Parse(string(data))
	if err != nil {
		e.Die(e.Wrap("iele", e.Bug, "main_parse", err))
	}
	f, err := os.Create(filepath.Join(name, "main.go"))
	if err != nil {
		e.Die(e.Wrap("iele", e.Trans, "main_create", err))
	}
	defer f.Close()
	if err := tmpl.Execute(f, map[string]string{
		"Name":   name,
		"Author": author,
		"Year":   fmt.Sprintf("%d", time.Now().Year()),
	}); err != nil {
		e.Die(e.Wrap("iele", e.Trans, "main_execute", err))
	}
}
