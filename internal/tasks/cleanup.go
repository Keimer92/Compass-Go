package tasks

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"compass/internal/project"
	"compass/internal/prompts"
	"compass/internal/shell"
	"compass/internal/theme"
	"compass/internal/tui"
	"compass/internal/ui"
)

func init() {
	for _, pt := range allTypes() {
		Register("clean", pt, CleanupManage)
		Register("clean-cache", pt, CleanupManage)
		Register("clean-project", pt, CleanupManage)

		// Acciones individuales para las subcards nativas de la GUI (mismo
		// estilo que Windows release o Firmas).
		for _, e := range cleanupEntries() {
			Register("clean-do-"+e.key, pt, e.run)
		}
	}
}

// CleanupOption es una accion del submenu de limpieza expuesta a la GUI.
type CleanupOption struct {
	Key         string
	Label       string
	Description string
	ColorKey    string
}

// CleanupOptions devuelve las acciones del submenu en el mismo orden que la
// TUI, para que la GUI pinte las mismas cards.
func CleanupOptions() []CleanupOption {
	entries := cleanupEntries()
	out := make([]CleanupOption, 0, len(entries))
	for _, e := range entries {
		out = append(out, CleanupOption{
			Key:         e.key,
			Label:       e.card.Label,
			Description: e.card.Description,
			ColorKey:    e.card.ColorKey,
		})
	}
	return out
}

// ── Utilidades ────────────────────────────────────────────────────────────────

func formatSize(bytes uint64) string {
	switch {
	case bytes > 1_000_000_000:
		return fmt.Sprintf("%.2f GB", float64(bytes)/1_073_741_824.0)
	case bytes > 1_000_000:
		return fmt.Sprintf("%.2f MB", float64(bytes)/1_048_576.0)
	default:
		return fmt.Sprintf("%.2f KB", float64(bytes)/1024.0)
	}
}

func dirSize(path string) uint64 {
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		return uint64(info.Size())
	}
	var total uint64
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		p := filepath.Join(path, e.Name())
		if e.IsDir() {
			total += dirSize(p)
		} else if info, err := e.Info(); err == nil {
			total += uint64(info.Size())
		}
	}
	return total
}

func purge(label, path string) uint64 {
	if !dirExists(path) {
		fmt.Printf("  [--] No hay %s\n", label)
		return 0
	}
	size := dirSize(path)
	var err error
	if info, _ := os.Stat(path); info != nil && info.IsDir() {
		err = shell.RemoveDir(path)
	} else {
		err = os.Remove(path)
	}
	if err != nil {
		fmt.Printf("  [WARN] No se pudo borrar %s (%v)\n", label, err)
		return 0
	}
	fmt.Printf("  [OK] %s (%s)\n", label, formatSize(size))
	return size
}

func findDirsNamed(root, name string) []string {
	var out []string
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > 8 || !dirExists(dir) {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			if !e.IsDir() {
				continue
			}
			switch e.Name() {
			case ".venv", "venv", ".git":
				continue
			}
			if e.Name() == name {
				out = append(out, p)
			} else {
				walk(p, depth+1)
			}
		}
	}
	walk(root, 0)
	return out
}

func moduleName(moduleDir string) string {
	return filepath.Base(moduleDir)
}

// ── Planes de limpieza ─────────────────────────────────────────────────────────

func quickTargets(ctx *project.ProjectContext) map[string]string {
	root := ctx.ScriptDir
	switch ctx.ProjectType {
	case project.TypeFlutter:
		return map[string]string{
			"build/":            filepath.Join(root, "build"),
			".dart_tool/":       filepath.Join(root, ".dart_tool"),
			"android/app/build": filepath.Join(root, "android", "app", "build"),
		}
	case project.TypeKotlin:
		module := kotlinAndroidDir(ctx)
		return map[string]string{
			"build/ raiz":                 filepath.Join(root, "build"),
			moduleName(module) + "/build": filepath.Join(module, "build"),
			".gradle/":                    filepath.Join(root, ".gradle"),
			".kotlin/":                    filepath.Join(root, ".kotlin"),
		}
	case project.TypeGo:
		return map[string]string{"bin/": filepath.Join(root, "bin")}
	case project.TypeGoWails:
		android := filepath.Join(root, "build", "android")
		return map[string]string{
			"bin/":              filepath.Join(root, "bin"),
			"frontend/dist":     filepath.Join(root, "frontend", "dist"),
			"Android app/build": filepath.Join(android, "app", "build"),
			"Android .gradle":   filepath.Join(android, ".gradle"),
		}
	case project.TypePython:
		targets := map[string]string{
			"build/": filepath.Join(root, "build"),
			"dist/":  filepath.Join(root, "dist"),
		}
		for _, name := range []string{"__pycache__", ".pytest_cache", ".mypy_cache", ".ruff_cache"} {
			for _, p := range findDirsNamed(root, name) {
				targets[name+"/"] = p
			}
		}
		return targets
	default:
		return map[string]string{}
	}
}

func fullTargets(ctx *project.ProjectContext) map[string]string {
	root := ctx.ScriptDir
	switch ctx.ProjectType {
	case project.TypeFlutter:
		return map[string]string{
			"build/":            filepath.Join(root, "build"),
			".dart_tool/":       filepath.Join(root, ".dart_tool"),
			"android/.gradle":   filepath.Join(root, "android", ".gradle"),
			"android/build":     filepath.Join(root, "android", "build"),
			"android/app/build": filepath.Join(root, "android", "app", "build"),
			"node_modules/":     filepath.Join(root, "node_modules"),
		}
	case project.TypeKotlin:
		module := kotlinAndroidDir(ctx)
		return map[string]string{
			"build/ raiz":                 filepath.Join(root, "build"),
			moduleName(module) + "/build": filepath.Join(module, "build"),
			".gradle/":                    filepath.Join(root, ".gradle"),
			".kotlin/":                    filepath.Join(root, ".kotlin"),
			"node_modules/":               filepath.Join(root, "node_modules"),
		}
	case project.TypeGo:
		return map[string]string{"bin/": filepath.Join(root, "bin")}
	case project.TypeGoWails:
		android := filepath.Join(root, "build", "android")
		return map[string]string{
			"bin/":                   filepath.Join(root, "bin"),
			"frontend/dist":          filepath.Join(root, "frontend", "dist"),
			"Android app/build":      filepath.Join(android, "app", "build"),
			"Android .gradle":        filepath.Join(android, ".gradle"),
			"frontend/node_modules/": filepath.Join(root, "frontend", "node_modules"),
		}
	case project.TypePython:
		targets := map[string]string{
			"build/": filepath.Join(root, "build"),
			"dist/":  filepath.Join(root, "dist"),
			".venv/": filepath.Join(root, ".venv"),
			"venv/":  filepath.Join(root, "venv"),
		}
		for _, name := range []string{"__pycache__", ".pytest_cache", ".mypy_cache", ".ruff_cache"} {
			for _, p := range findDirsNamed(root, name) {
				targets[name+"/"] = p
			}
		}
		return targets
	default:
		return map[string]string{}
	}
}

type candidate struct {
	label string
	path  string
	deps  bool
}

func selectableTargets(ctx *project.ProjectContext) []candidate {
	root := ctx.ScriptDir
	c := func(label, path string, deps bool) candidate {
		return candidate{label: label, path: path, deps: deps}
	}
	switch ctx.ProjectType {
	case project.TypeFlutter:
		return []candidate{
			c("build/", filepath.Join(root, "build"), false),
			c(".dart_tool/", filepath.Join(root, ".dart_tool"), false),
			c("android/.gradle", filepath.Join(root, "android", ".gradle"), false),
			c("android/build", filepath.Join(root, "android", "build"), false),
			c("android/app/build", filepath.Join(root, "android", "app", "build"), false),
			c("node_modules/", filepath.Join(root, "node_modules"), true),
		}
	case project.TypeKotlin:
		module := kotlinAndroidDir(ctx)
		return []candidate{
			c("build/ raiz", filepath.Join(root, "build"), false),
			c(moduleName(module)+"/build", filepath.Join(module, "build"), false),
			c(".gradle/", filepath.Join(root, ".gradle"), false),
			c(".kotlin/", filepath.Join(root, ".kotlin"), false),
			c("node_modules/", filepath.Join(root, "node_modules"), true),
		}
	case project.TypeGo:
		return []candidate{c("bin/", filepath.Join(root, "bin"), false)}
	case project.TypeGoWails:
		android := filepath.Join(root, "build", "android")
		return []candidate{
			c("bin/", filepath.Join(root, "bin"), false),
			c("frontend/dist", filepath.Join(root, "frontend", "dist"), false),
			c("Android app/build", filepath.Join(android, "app", "build"), false),
			c("Android .gradle", filepath.Join(android, ".gradle"), false),
			c("frontend/node_modules/", filepath.Join(root, "frontend", "node_modules"), true),
		}
	case project.TypePython:
		var targets []candidate
		for _, t := range []struct {
			label string
			path  string
			deps  bool
		}{
			{"build/", filepath.Join(root, "build"), false},
			{"dist/", filepath.Join(root, "dist"), false},
			{".venv/", filepath.Join(root, ".venv"), true},
			{"venv/", filepath.Join(root, "venv"), true},
		} {
			targets = append(targets, c(t.label, t.path, t.deps))
		}
		for _, p := range findDirsNamed(root, "__pycache__") {
			targets = append(targets, c("__pycache__/", p, false))
		}
		for _, p := range findDirsNamed(root, ".pytest_cache") {
			targets = append(targets, c(".pytest_cache/", p, false))
		}
		for _, p := range findDirsNamed(root, ".mypy_cache") {
			targets = append(targets, c(".mypy_cache/", p, false))
		}
		for _, p := range findDirsNamed(root, ".ruff_cache") {
			targets = append(targets, c(".ruff_cache/", p, false))
		}
		return targets
	default:
		return nil
	}
}

// ── Acciones ──────────────────────────────────────────────────────────────────

func quickClean(ctx *project.ProjectContext) error {
	shell.ClearScreen()
	ui.BoxBanner("LIMPIEZA RAPIDA")
	fmt.Println()
	fmt.Println("Borra builds y caches regenerables; conserva node_modules/ y")
	fmt.Println("las dependencias. Nada que tardes minutos en reinstalar.")
	fmt.Println()

	var freed uint64
	for label, path := range orderedTargets(quickTargets(ctx)) {
		freed += purge(label, path)
	}
	fmt.Println()
	fmt.Printf("[OK] Limpieza rapida completada. Liberado: %s\n", formatSize(freed))
	ui.Pause()
	return nil
}

func fullClean(ctx *project.ProjectContext) error {
	switch ctx.ProjectType {
	case project.TypeFlutter:
		return FlutterClean(ctx)
	case project.TypeKotlin:
		return KotlinClean(ctx)
	case project.TypeGo:
		return GoClean(ctx)
	case project.TypeGoWails:
		return GoWailsClean(ctx)
	case project.TypePython:
		return PythonClean(ctx)
	default:
		return nil
	}
}

func reportSizes(ctx *project.ProjectContext) error {
	shell.ClearScreen()
	ui.BoxBanner("ESPACIO QUE SE PUEDE LIBERAR")
	fmt.Println()
	fmt.Print("Nada se borra aqui: solo se mide.\n\n")

	fmt.Println("-- Rapida --------------------------------------------")
	var quickTotal uint64
	for label, path := range orderedTargets(quickTargets(ctx)) {
		size := dirSize(path)
		if dirExists(path) {
			fmt.Printf("  %-26s %10s\n", label, formatSize(size))
			quickTotal += size
		} else {
			fmt.Printf("  %-26s %10s\n", label, "—")
		}
	}
	fmt.Printf("  %-26s %10s\n", "SUBTOTAL", formatSize(quickTotal))

	fmt.Println()
	fmt.Println("-- Completa ------------------------------------------")
	var fullTotal uint64
	for label, path := range orderedTargets(fullTargets(ctx)) {
		size := dirSize(path)
		if dirExists(path) {
			fmt.Printf("  %-26s %10s\n", label, formatSize(size))
			fullTotal += size
		} else {
			fmt.Printf("  %-26s %10s\n", label, "—")
		}
	}
	fmt.Printf("  %-26s %10s\n", "TOTAL", formatSize(fullTotal))

	fmt.Println()
	fmt.Println("[TIP] La rapida conserva dependencias; la completa las borra, asi que")
	fmt.Println("      restaurarlas despues (venv / npm install / pub get) tarda mas.")
	ui.Pause()
	return nil
}

func chooseAndPurge(ctx *project.ProjectContext) error {
	shell.ClearScreen()
	ui.BoxBanner("ELEGIR QUE BORRAR")
	fmt.Println()
	fmt.Println("Marca con Espacio lo que quieras eliminar y confirma con Enter.")
	fmt.Println()

	candidates := selectableTargets(ctx)
	var existing []candidate
	for _, cnd := range candidates {
		if dirExists(cnd.path) {
			existing = append(existing, cnd)
		}
	}
	if len(existing) == 0 {
		fmt.Println("[INFO] No hay nada que borrar: no existe ningun directorio de build.")
		ui.Pause()
		return nil
	}

	for i, cnd := range existing {
		tag := "build"
		if cnd.deps {
			tag = "deps "
		}
		fmt.Printf("  %2d. %-24s %10s  [%s]\n", i+1, cnd.label, formatSize(dirSize(cnd.path)), tag)
	}
	fmt.Println()
	fmt.Print("Numeros a borrar (separados por coma, o Enter para cancelar): ")
	var line string
	fmt.Scanln(&line)
	line = strings.TrimSpace(line)
	if line == "" {
		fmt.Println("\n[INFO] No seleccionaste nada.")
		ui.Pause()
		return nil
	}

	var chosen []candidate
	for _, part := range strings.Split(line, ",") {
		part = strings.TrimSpace(part)
		n, err := strconvAtoiSafe(part)
		if err != nil || n < 1 || n > len(existing) {
			continue
		}
		chosen = append(chosen, existing[n-1])
	}
	if len(chosen) == 0 {
		fmt.Println("\n[INFO] Seleccion no valida.")
		ui.Pause()
		return nil
	}

	var deps []candidate
	for _, c := range chosen {
		if c.deps {
			deps = append(deps, c)
		}
	}
	if len(deps) > 0 {
		fmt.Println("\n[AVISO] Vas a borrar dependencias; reinstalarlas tardara:")
		for _, c := range deps {
			fmt.Printf("  - %s\n", c.label)
		}
		ok, _ := confirmCLI("¿Continuar?")
		if !ok {
			fmt.Println("[INFO] Operacion cancelada.")
			ui.Pause()
			return nil
		}
	}

	fmt.Println()
	var freed uint64
	for _, c := range chosen {
		freed += purge(c.label, c.path)
	}
	fmt.Println()
	fmt.Printf("[OK] Limpieza selectiva completada. Liberado: %s\n", formatSize(freed))
	ui.Pause()
	return nil
}

func confirmCLI(prompt string) (bool, error) {
	return prompts.Confirm(prompt, false)
}

// ── Submenu de limpieza (cards) ─────────────────────────────────────────────

type cleanupItem struct {
	key  string
	card tui.SubItem
	run  func(*project.ProjectContext) error
}

func cleanupEntries() []cleanupItem {
	return []cleanupItem{
		{"quick", tui.SubItem{
			Icon: "\u26a1", Label: "Rápida",
			Description: "Builds y caches · conserva deps", ColorKey: "clean",
		}, quickClean},
		{"full", tui.SubItem{
			Icon: "\U0001f5d1", Label: "Completa",
			Description: "Borra también dependencias", ColorKey: "build",
		}, fullClean},
		{"choose", tui.SubItem{
			Icon: "\U0001f3af", Label: "Elegir qué borrar",
			Description: "Lista y marca qué borrar", ColorKey: "debug",
		}, chooseAndPurge},
		{"sizes", tui.SubItem{
			Icon: "\U0001f4ca", Label: "Ver tamaños",
			Description: "Mide sin borrar nada", ColorKey: "profile",
		}, reportSizes},
	}
}

// CleanupManage es el punto de entrada de las tareas clean/clean-cache/clean-project.
//
// Tras ejecutar una accion se vuelve al submenu, como en Rust.
func CleanupManage(ctx *project.ProjectContext) error {
	entries := cleanupEntries()
	cards := make([]tui.SubEntry[string], len(entries))
	for i, e := range entries {
		cards[i] = tui.SubEntry[string]{Value: e.key, Item: e.card}
	}
	menu := tui.NewSubmenu("LIMPIEZA", "MNT", cards)
	th := theme.Load()

	for {
		choice, ok, err := menu.Run(ctx, th)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		for _, e := range entries {
			if e.key == choice {
				if e.run != nil {
					if err := e.run(ctx); err != nil {
						return err
					}
				}
				break
			}
		}
	}
}

// orderedTargets devuelve las claves en orden estable (alfabetico) con su valor.
func orderedTargets(m map[string]string) map[string]string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(m))
	for _, k := range keys {
		out[k] = m[k]
	}
	return out
}

func strconvAtoiSafe(s string) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("no es digito")
		}
		n = n*10 + int(r-'0')
	}
	if s == "" {
		return 0, fmt.Errorf("vacio")
	}
	return n, nil
}
