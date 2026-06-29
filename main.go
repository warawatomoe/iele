package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"iele/internal/arg"
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
	var (
		showHelp    bool
		showVersion bool
		name        string
		author      string
		license     string = "mit"
		pkgs        string
	)

	opts := []arg.Opt{
		{Key: 'h', Typ: arg.TFlg, Dst: &showHelp, Doc: "show help"},
		{Key: 'v', Typ: arg.TFlg, Dst: &showVersion, Doc: "print version"},
		{Key: 'n', Typ: arg.TStr, Dst: &name, Met: "name", Doc: "project name", Flg: arg.Req},
		{Key: 'a', Typ: arg.TStr, Dst: &author, Met: "author", Doc: "name of the author", Flg: arg.Req},
		{Key: 'l', Typ: arg.TStr, Dst: &license, Met: "license", Doc: "mit, bsd-2, apache-2 (default: mit)"},
		{Key: 'p', Typ: arg.TStr, Dst: &pkgs, Met: "packages", Doc: "comma-separated optional packages"},
	}

	usage := arg.Usage(opts, nil)
	arg.Parse("iele", usage, opts)

	if showHelp {
		arg.Help(os.Stdout, "iele", usage, opts)
		os.Exit(0)
	}

	if showVersion {
		fmt.Printf("iele %s\n", version)
		os.Exit(0)
	}

	if name == "" || author == "" {
		arg.Help(os.Stderr, "iele", usage, opts)
		e.Die(e.New("iele", e.Call, "main", "name_and_author_required"))
	}

	licFile, ok := licNames[license]
	if !ok {
		e.Die(e.New("iele", e.Call, "main", "invalid_license"))
	}

	selected := make(map[string]bool)
	for _, m := range mandatory {
		selected[m] = true
	}
	if pkgs != "" {
		for _, p := range strings.Split(pkgs, ",") {
			p = strings.TrimSpace(p)
			if !optional[p] {
				e.Die(e.New("iele", e.Call, "main", "unknown_pkg_"+p))
			}
			selected[p] = true
		}
	}

	if err := os.MkdirAll(name, 0755); err != nil {
		e.Die(e.Wrap("iele", e.Trans, "mkdir", err))
	}

	for pkg := range selected {
		copyPkg(name, pkg)
	}

	stampLicense(name, licFile, author)
	stampMain(name, author)

	gomod := fmt.Sprintf("module %s\n\ngo 1.22\n", name)
	if err := os.WriteFile(filepath.Join(name, "go.mod"), []byte(gomod), 0644); err != nil {
		e.Die(e.Wrap("iele", e.Trans, "gomod", err))
	}

	fmt.Printf("stamped %s\n", name)
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
