package tasks

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"compass/internal/project"
	"compass/internal/shell"
	"compass/internal/ui"
)

func init() {
	Register("version", project.TypeFlutter, VersionBump)
	Register("version", project.TypeKotlin, VersionBump)
	Register("version", project.TypePython, VersionBump)
	Register("version", project.TypeGo, VersionBump)
	Register("version", project.TypeGoWails, VersionBump)
	// Los corekits tambien se versionan: su version vive en el package.json
	// (Node), igual que en un proyecto JS normal.
	Register("version", project.TypeWhatsappKit, VersionBump)
	Register("version", project.TypeKitD1, VersionBump)
}

type versionFormat int

const (
	fmtJSON versionFormat = iota
	fmtTOML
	fmtYAML
	fmtPyproject
	fmtGoConst
	fmtNsis
	fmtInfoJSON
	fmtGradle
	fmtPlist
	fmtPubspec
	fmtWinres
	fmtRunnerRC
	// fmtGradleProject es la `version = "x.y.z"` de un build.gradle(.kts) sin
	// versionName/versionCode (Kotlin Multiplatform, desktop, librerias).
	fmtGradleProject
)

type versionSource int

const (
	srcRequired versionSource = iota
	srcOptional
)

type sourceFile struct {
	path   string
	format versionFormat
	source versionSource
}

type versionFile struct {
	path        string
	format      versionFormat
	current     string
	hasCurrent  bool
	currentCode uint64
	hasCode     bool
	text        string
}

var (
	reJSON           = regexp.MustCompile(`"version"\s*:\s*"([^"]+)"`)
	reTOML           = regexp.MustCompile(`(?m)^version\s*=\s*"([^"]+)"`)
	reYAML           = regexp.MustCompile(`(?m)^[ \t]*version:[ \t]*"([^"]+)"`)
	reYAMLReplace    = regexp.MustCompile(`(?m)^([ \t]*)version:[ \t]*"[^"]+"`)
	reGoConst        = regexp.MustCompile(`(?m)^[ \t]*const[ \t]+version[ \t]*=[ \t]*"([^"]+)"`)
	reGoConstReplace = regexp.MustCompile(`(?m)^([ \t]*const[ \t]+version[ \t]*=[ \t]*")[^"]+(")`)
	reNsis           = regexp.MustCompile(`(?m)^!define[ \t]+INFO_PRODUCTVERSION[ \t]+"([^"]+)"`)
	reNsisReplace    = regexp.MustCompile(`(?m)^(!define[ \t]+INFO_PRODUCTVERSION[ \t]+")[^"]+(")`)
	rePlist          = regexp.MustCompile(`(?ms)(<key>CFBundle(?:ShortVersionString|Version)</key>\s*<string>)[^<]*?(</string>)`)
	rePlistFirst     = regexp.MustCompile(`(?ms)<key>CFBundleShortVersionString</key>\s*<string>([^<]*)</string>`)
	reIJFile         = regexp.MustCompile(`"file_version"\s*:\s*"([^"]+)"`)
	reIJProduct      = regexp.MustCompile(`"product_version"\s*:\s*"([^"]+)"`)
	reIJProductCap   = regexp.MustCompile(`"ProductVersion"\s*:\s*"([^"]+)"`)
	reGradleName     = regexp.MustCompile(`(?m)^[ \t]*versionName[ \t]*=?[ \t]*"([^"]+)"`)
	reGradleCode     = regexp.MustCompile(`(?m)^[ \t]*versionCode[ \t]*=?[ \t]*(\d+)`)
	reGradleNameRep  = regexp.MustCompile(`(?m)^([ \t]*versionName[ \t]*=?[ \t]*")[^"]+(")`)
	reGradleCodeRep  = regexp.MustCompile(`(?m)^([ \t]*versionCode[ \t]*=?[ \t]*)(\d+)`)
	// version = "x.y.z" del proyecto Gradle (KMP/desktop), sin versionName.
	// No confunde `versionName = "..."` porque exige `=` justo tras `version`.
	reGradleProjVer    = regexp.MustCompile(`(?m)^[ \t]*version[ \t]*=[ \t]*"([^"]+)"`)
	reGradleProjVerRep = regexp.MustCompile(`(?m)^([ \t]*version[ \t]*=[ \t]*")[^"]+(")`)
	rePyprojectVer     = regexp.MustCompile(`(?m)^([ \t]*version[ \t]*=[ \t]*")[^"]+(".*)$`)
	rePackage          = regexp.MustCompile(`(?m)^\[package\]\s*$`)
	reNewVersion       = regexp.MustCompile(`^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$`)

	// Flutter: version: 1.2.3+45 (build number despues del '+').
	rePubspec        = regexp.MustCompile(`(?m)^[ \t]*version:[ \t]*(\d+\.\d+\.\d+)(?:\+(\d+))?[ \t]*$`)
	rePubspecReplace = regexp.MustCompile(`(?m)^([ \t]*version:[ \t]*)(\d+\.\d+\.\d+)(?:\+\d+)?([ \t]*)$`)
	// go-winres.json (Wails v3): todos los campos de version.
	reWinresIdentity = regexp.MustCompile(`("version"[ \t]*:[ \t]*")[^"]+(")`)
	reWinresFixed    = regexp.MustCompile(`("(?:file_version|product_version)"[ \t]*:[ \t]*")[^"]+(")`)
	reWinresInfo     = regexp.MustCompile(`("(?:FileVersion|ProductVersion)"[ \t]*:[ \t]*")[^"]+(")`)
	// Flutter Windows: valores fallback de Runner.rc cuando no hay defines.
	reRCStr = regexp.MustCompile(`(?m)^([ \t]*#define[ \t]+VERSION_AS_STRING[ \t]+")[^"]+(")`)
	// reRCStrVal captura SOLO el valor; reRCStr tiene el prefijo en el grupo 1
	// porque se usa para reemplazar, no para leer.
	reRCStrVal = regexp.MustCompile(`(?m)^[ \t]*#define[ \t]+VERSION_AS_STRING[ \t]+"([^"]+)"`)
	reRCNum    = regexp.MustCompile(`(?m)^([ \t]*#define[ \t]+VERSION_AS_NUMBER[ \t]+)\d+,\d+,\d+,\d+`)
)

// VersionBump actualiza la version del proyecto (port de versionar.bat).
// Equivalente de tasks/version.rs::bump.
func VersionBump(ctx *project.ProjectContext) error {
	printVersionBanner()

	plan, err := planVersion(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("  Tipo de proyecto: %s\n", projectTypeLabel(ctx.ProjectType))
	fmt.Println()
	for _, o := range plan.omitted {
		rel, _ := filepath.Rel(ctx.ScriptDir, o)
		fmt.Printf("  (no existe, se omite)   %s\n", rel)
	}
	for _, f := range plan.files {
		rel, _ := filepath.Rel(ctx.ScriptDir, f.path)
		cur := f.current
		if !f.hasCurrent {
			cur = "(sin version - se agregara)"
		}
		fmt.Printf("  %-34s : %s\n", rel, cur)
	}
	fmt.Println()

	newVer := ui.AskText("Nueva version (X.Y.Z, opcional sufijo -pre/+build)", "")
	newVer = strings.TrimSpace(newVer)
	if newVer == "" {
		return fmt.Errorf("version vacia")
	}
	if !ValidVersion(newVer) {
		return fmt.Errorf("formato invalido. Usa X.Y.Z (ej: 1.2.3) o con sufijo (ej: 2.0.0-beta.1)")
	}

	added, gradleNotes, err := plan.Apply(newVer)
	if err != nil {
		return err
	}

	fmt.Printf("[OK] Version actualizada a %s en %d archivo(s).\n", newVer, len(plan.files))
	if added > 0 {
		fmt.Printf("[OK] Se inserto el campo version en %d archivo(s) que no lo tenian.\n", added)
	}
	for _, n := range gradleNotes {
		fmt.Printf("[OK] versionCode incrementado en %s\n", n)
	}
	fmt.Println()
	ui.Pause()
	return nil
}

// ValidVersion indica si s tiene el formato X.Y.Z con sufijo opcional.
func ValidVersion(s string) bool {
	return reNewVersion.MatchString(strings.TrimSpace(s))
}

// VersionFileInfo describe un archivo que el versionador tocaria. Es lo que
// pinta el panel GUI antes de aplicar el cambio.
type VersionFileInfo struct {
	Path    string `json:"path"`
	Current string `json:"current"`
	Missing bool   `json:"missing"` // no tiene marcador: se insertara
	Code    uint64 `json:"code"`    // versionCode de Gradle (0 si no aplica)
	HasCode bool   `json:"hasCode"`
}

// VersionPlanInfo es el plan completo de un proyecto.
type VersionPlanInfo struct {
	Type    string            `json:"type"`
	Files   []VersionFileInfo `json:"files"`
	Omitted []string          `json:"omitted"`
}

// VersionResult resume lo aplicado, para el mensaje final de la GUI.
type VersionResult struct {
	Version string   `json:"version"`
	Files   int      `json:"files"`
	Added   int      `json:"added"`
	Notes   []string `json:"notes"`
}

// VersionInfo calcula el plan sin escribir nada: lista los archivos de version
// del proyecto y el valor actual de cada uno.
func VersionInfo(ctx *project.ProjectContext) (VersionPlanInfo, error) {
	plan, err := planVersion(ctx)
	if err != nil {
		return VersionPlanInfo{}, err
	}
	info := VersionPlanInfo{
		Type:    projectTypeLabel(ctx.ProjectType),
		Files:   make([]VersionFileInfo, 0, len(plan.files)),
		Omitted: make([]string, 0, len(plan.omitted)),
	}
	for _, o := range plan.omitted {
		if rel, err := filepath.Rel(ctx.ScriptDir, o); err == nil {
			info.Omitted = append(info.Omitted, rel)
		}
	}
	for _, f := range plan.files {
		rel, err := filepath.Rel(ctx.ScriptDir, f.path)
		if err != nil {
			rel = f.path
		}
		info.Files = append(info.Files, VersionFileInfo{
			Path:    rel,
			Current: f.current,
			Missing: !f.hasCurrent,
			Code:    f.currentCode,
			HasCode: f.hasCode,
		})
	}
	return info, nil
}

// VersionApply valida y aplica la version nueva sin preguntar nada. Lo usa el
// panel GUI; la tarea de terminal comparte el mismo plan y la misma escritura.
func VersionApply(ctx *project.ProjectContext, newVer string) (VersionResult, error) {
	newVer = strings.TrimSpace(newVer)
	if newVer == "" {
		return VersionResult{}, fmt.Errorf("version vacia")
	}
	if !ValidVersion(newVer) {
		return VersionResult{}, fmt.Errorf("formato invalido. Usa X.Y.Z (ej: 1.2.3) o con sufijo (ej: 2.0.0-beta.1)")
	}
	plan, err := planVersion(ctx)
	if err != nil {
		return VersionResult{}, err
	}
	added, notes, err := plan.Apply(newVer)
	if err != nil {
		return VersionResult{}, err
	}
	return VersionResult{
		Version: newVer,
		Files:   len(plan.files),
		Added:   added,
		// En el modal las rutas absolutas no caben: se muestran relativas.
		Notes: relVersionNotes(ctx.ScriptDir, notes),
	}, nil
}

// relVersionNotes quita la raiz del proyecto de las notas de versionCode.
func relVersionNotes(root string, notes []string) []string {
	prefix := strings.TrimRight(root, `\/`) + string(filepath.Separator)
	out := make([]string, len(notes))
	for i, n := range notes {
		out[i] = strings.ReplaceAll(n, prefix, "")
	}
	return out
}

// versionPlan es el resultado de la fase de validacion (todo-o-nada): que
// archivos se tocarian y con que version actual.
type versionPlan struct {
	files   []versionFile
	omitted []string
}

// planVersion valida el proyecto y devuelve sus archivos de version. No
// escribe nada: si algo falta, el error sale antes de tocar disco.
func planVersion(ctx *project.ProjectContext) (*versionPlan, error) {
	// Go puro guarda la version en un `const version` de un archivo Go. No se
	// puede usar go.mod: una directiva `version = "..."` rompe el build con
	// "unknown directive: version".
	if ctx.ProjectType == project.TypeGo {
		return planGoVersion(ctx)
	}

	configs := versionConfigs(ctx)
	var files []versionFile
	var omitted []string
	for _, sf := range configs {
		if !fileExists(sf.path) {
			if sf.source == srcRequired {
				return nil, fmt.Errorf("no existe el archivo: %s", sf.path)
			}
			omitted = append(omitted, sf.path)
			continue
		}
		data, err := os.ReadFile(sf.path)
		if err != nil {
			return nil, fmt.Errorf("no se pudo leer %s: %w", sf.path, err)
		}
		text := strings.TrimPrefix(string(data), "\ufeff")

		var cur string
		var hasCur bool
		if sf.format == fmtGradle {
			cc, hasCode := gradleCode(text)
			cur, hasCur = gradleName(text)
			if hasCur && !hasCode {
				// Kotlin DSL/Gradle sin versionCode: no se puede mantener el
				// entero sincronizado. Si es fuente obligatoria, avisamos; si no,
				// la omitimos (p. ej. build.gradle de un modulo que no aplica).
				if sf.source == srcRequired {
					return nil, fmt.Errorf("no se encontro `versionCode` en: %s\n  Anadelo dentro del bloque defaultConfig.", sf.path)
				}
				hasCur = false
			}
			if hasCur {
				files = append(files, versionFile{path: sf.path, format: sf.format, current: cur, hasCurrent: true, currentCode: cc, hasCode: true, text: text})
				continue
			}
			// Sin versionName: puede ser un build.gradle de Kotlin Multiplatform
			// o desktop, donde la version es `version = "x.y.z"` del proyecto.
			if v, ok := cap1(reGradleProjVer, text); ok {
				files = append(files, versionFile{path: sf.path, format: fmtGradleProject, current: v, hasCurrent: true, text: text})
				continue
			}
		} else {
			cur, hasCur = detectVersion(sf.format, text)
		}

		if hasCur {
			files = append(files, versionFile{path: sf.path, format: sf.format, current: cur, hasCurrent: true, text: text})
			continue
		}

		// Sin marcador: una fuente opcional que no sabemos insertar con
		// seguridad se omite en lugar de abortar todo el versionado.
		//
		// fmtGoConst es insertable, pero en Wails main.go se omite igual: la
		// version canonica vive en build/config.yml y frontend/package.json, y
		// añadir un `const version` que nadie lee solo ensucia el codigo. Si el
		// proyecto ya lo tiene, hasCur es true y se actualiza como cualquier
		// otro archivo.
		if sf.source == srcOptional && (sf.format == fmtGoConst || !isInsertableFormat(sf.format)) {
			omitted = append(omitted, sf.path)
			continue
		}

		if sf.format == fmtPyproject {
			return nil, fmt.Errorf("no se encontró una versión estática dentro de `[project]` o `[tool.poetry]` en %s. Los proyectos con version dinámica deben actualizarla en su sistema de build.", sf.path)
		}
		if sf.format == fmtYAML {
			return nil, fmt.Errorf("no se encontro el campo `version:` en: %s\n  Añádelo bajo `info:` y vuelve a intentarlo.", sf.path)
		}
		if sf.format != fmtJSON && sf.format != fmtTOML {
			return nil, fmt.Errorf("no se encontro el marcador de version en: %s\n  %s", sf.path, missingHint(sf.format))
		}
		if sf.format == fmtTOML {
			if !rePackage.MatchString(text) {
				return nil, fmt.Errorf("TOML sin version y sin seccion [package]:\n  no se puede insertar de forma segura: %s", sf.path)
			}
		} else {
			if !strings.HasPrefix(strings.TrimSpace(text), "{") {
				// Validación ligera de objeto JSON raíz.
				if !jsonIsObject(text) {
					return nil, fmt.Errorf("JSON no es un objeto; no se puede insertar version de forma segura: %s", sf.path)
				}
			}
		}
		files = append(files, versionFile{path: sf.path, format: sf.format, hasCurrent: false, text: text})
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no se encontro ningun archivo de version reconocido para %s", projectTypeLabel(ctx.ProjectType))
	}
	return &versionPlan{files: files, omitted: omitted}, nil
}

// Apply escribe la version nueva en todos los archivos del plan y devuelve
// cuantos campos se insertaron y las notas de versionCode.
func (p *versionPlan) Apply(newVer string) (int, []string, error) {
	var added int
	var gradleNotes []string
	for i := range p.files {
		f := &p.files[i]
		var newText string
		if f.hasCurrent {
			if f.format == fmtGradle {
				gradleNotes = append(gradleNotes, fmt.Sprintf("%s -> %d", f.path, f.currentCode+1))
			}
			var err error
			newText, err = applyFormat(f, newVer)
			if err != nil {
				return added, gradleNotes, fmt.Errorf("%s: %w", f.path, err)
			}
		} else {
			added++
			if f.format == fmtTOML {
				newText = rePackage.ReplaceAllString(f.text, fmt.Sprintf("[package]\nversion = \"%s\"", newVer))
			} else if f.format == fmtGoConst {
				newText = applyGoVersion(f.text, false, newVer)
			} else {
				idx := strings.Index(f.text, "{")
				if idx < 0 {
					return added, gradleNotes, fmt.Errorf("no se encontro el inicio del objeto JSON: %s", f.path)
				}
				prop := fmt.Sprintf("\"version\": \"%s\"", newVer)
				rest := f.text[idx+1:]
				if strings.TrimSpace(rest) == "}" {
					newText = f.text[:idx+1] + prop + f.text[idx+1:]
				} else {
					newText = f.text[:idx+1] + prop + ", " + f.text[idx+1:]
				}
			}
		}
		if err := os.WriteFile(f.path, []byte(newText), 0644); err != nil {
			return added, gradleNotes, fmt.Errorf("no se pudo escribir %s: %w", f.path, err)
		}
	}
	return added, gradleNotes, nil
}

func printVersionBanner() {
	shell.ClearScreen()
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║              VERSIONADOR DE PROYECTO              ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()
}

// goVersionFile elige el archivo Go de la raiz que guarda `const version`.
// Preferimos main.go (lo habitual); si no existe, usamos version.go (que se
// creara con un package valido deducido del module de go.mod).
func goVersionFile(root string) string {
	mainGo := filepath.Join(root, "main.go")
	if fileExists(mainGo) {
		return mainGo
	}
	return filepath.Join(root, "version.go")
}

// goPackageFromModule deduce un nombre de package Go valido desde la directiva
// `module` de go.mod (parte final, caracteres no validos -> '_').
func goPackageFromModule(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "main"
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "module ") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(line, "module "))
		base := name
		if i := strings.LastIndexAny(base, "/"); i >= 0 {
			base = base[i+1:]
		}
		var b strings.Builder
		for _, ch := range base {
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' ||
				(ch >= '0' && ch <= '9' && b.Len() > 0) {
				b.WriteRune(ch)
			} else {
				b.WriteRune('_')
			}
		}
		if b.Len() > 0 {
			return b.String()
		}
	}
	return "main"
}

// planGoVersion prepara el archivo Go que guarda `const version`. Si no existe
// el archivo, se parte de un `package` valido y Apply inserta el const.
func planGoVersion(ctx *project.ProjectContext) (*versionPlan, error) {
	path := goVersionFile(ctx.ScriptDir)
	var text string
	if fileExists(path) {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("no se pudo leer %s: %w", path, err)
		}
		text = strings.TrimPrefix(string(data), "\ufeff")
	} else {
		text = fmt.Sprintf("package %s\n", goPackageFromModule(ctx.ScriptDir))
	}

	cur, hasCur := detectVersion(fmtGoConst, text)
	return &versionPlan{files: []versionFile{{
		path:       path,
		format:     fmtGoConst,
		current:    cur,
		hasCurrent: hasCur,
		text:       text,
	}}}, nil
}

// applyGoVersion reemplaza el `const version` existente o lo inserta en un
// punto seguro cuando el archivo aun no lo tiene: despues del bloque de
// imports (o tras el `package` si no hay imports), para no dejar un `import`
// despues de una declaracion, que seria codigo Go invalido.
func applyGoVersion(text string, hasCur bool, newVer string) string {
	if hasCur {
		return reGoConstReplace.ReplaceAllString(text, fmt.Sprintf(`${1}%s${2}`, newVer))
	}
	lines := strings.SplitAfter(text, "\n")
	at := len(lines)
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "package ") {
			at = i + 1
			break
		}
	}
	// Si hay imports, el const va despues del ultimo bloque `import`.
	inImport := false
	lastImport := at
	for i := at; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if !inImport {
			// `import "fmt"` es de una linea; `import (` abre un bloque.
			if strings.HasPrefix(t, "import ") {
				lastImport = i + 1
				if strings.Contains(t, "(") && !strings.Contains(t, ")") {
					inImport = true
				}
			}
			continue
		}
		// Dentro de un bloque de imports, hasta el `)` de cierre.
		lastImport = i + 1
		if strings.HasPrefix(t, ")") {
			inImport = false
		}
	}
	if lastImport > at {
		at = lastImport
	}
	block := []string{"\n// version es la version de la app, gestionada por Compass Go.\n",
		fmt.Sprintf("const version = %q\n", newVer)}
	lines = append(lines[:at], append(block, lines[at:]...)...)
	return strings.Join(lines, "")
}

func versionConfigs(ctx *project.ProjectContext) []sourceFile {
	root := ctx.ScriptDir
	mk := func(rel string, f versionFormat, s versionSource) sourceFile {
		return sourceFile{path: filepath.Join(root, rel), format: f, source: s}
	}
	switch ctx.ProjectType {
	case project.TypeFlutter:
		// La fuente canonica es pubspec.yaml (version: X.Y.Z+B). El resto se
		// deriva de ella en tiempo de build, pero se sincroniza si existe.
		return []sourceFile{
			mk("pubspec.yaml", fmtPubspec, srcRequired),
			mk("android/app/build.gradle", fmtGradle, srcOptional),
			mk("android/app/build.gradle.kts", fmtGradle, srcOptional),
			mk("ios/Runner/Info.plist", fmtPlist, srcOptional),
			mk("ios/Flutter/Info.plist", fmtPlist, srcOptional),
			mk("macos/Runner/Info.plist", fmtPlist, srcOptional),
			mk("windows/runner/Runner.rc", fmtRunnerRC, srcOptional),
		}
	case project.TypeKotlin:
		// Kotlin Multiplatform: build.gradle(.kts) en la raiz y en los modulos.
		// Android usa `versionName` + `versionCode` (Groovy o Kotlin DSL); KMP y
		// desktop usan `version = "x.y.z"` en la raiz (fmtGradleProject).
		return []sourceFile{
			mk("build.gradle", fmtGradle, srcOptional),
			mk("build.gradle.kts", fmtGradle, srcOptional),
			mk("app/build.gradle", fmtGradle, srcOptional),
			mk("app/build.gradle.kts", fmtGradle, srcOptional),
			mk("composeApp/build.gradle", fmtGradle, srcOptional),
			mk("composeApp/build.gradle.kts", fmtGradle, srcOptional),
		}
	case project.TypeGoWails:
		// Wails v3 no usa build/config.yml: la version vive en go-winres.json
		// y en frontend/package.json. Se conservan las rutas v1/v2 como
		// opcionales para proyectos antiguos; ninguna es obligatoria.
		return []sourceFile{
			mk("go-winres.json", fmtWinres, srcOptional),
			mk("frontend/package.json", fmtJSON, srcOptional),
			mk("main.go", fmtGoConst, srcOptional),
			mk("build/config.yml", fmtYAML, srcOptional),
			mk("build/windows/nsis/project.nsi", fmtNsis, srcOptional),
			mk("build/windows/info.json", fmtInfoJSON, srcOptional),
			mk("build/android/app/build.gradle", fmtGradle, srcOptional),
			mk("build/android/custom/app/build.gradle", fmtGradle, srcOptional),
			mk("build/ios/Info.plist", fmtPlist, srcOptional),
			mk("build/ios/Info.dev.plist", fmtPlist, srcOptional),
			mk("build/darwin/Info.plist", fmtPlist, srcOptional),
			mk("build/darwin/Info.dev.plist", fmtPlist, srcOptional),
			mk("build/linux/nfpm/nfpm.yaml", fmtYAML, srcOptional),
		}
	case project.TypePython:
		return []sourceFile{mk("pyproject.toml", fmtPyproject, srcRequired)}
	case project.TypeWhatsappKit, project.TypeKitD1:
		// Los corekits son paquetes Node: la version vive en el package.json del
		// kit (no en el de node_modules ni en el de las instancias generadas).
		// Kit WhatsApp la usa tambien para publicar el motor; Kit D1 la declara
		// aqui aunque su codigo sea Go + scripts .mjs.
		return []sourceFile{mk("package.json", fmtJSON, srcRequired)}
	default: // Go
		return []sourceFile{mk("go.mod", fmtTOML, srcRequired)}
	}
}

func detectVersion(f versionFormat, text string) (string, bool) {
	switch f {
	case fmtJSON:
		return cap1(reJSON, text)
	case fmtTOML:
		return cap1(reTOML, text)
	case fmtYAML:
		return cap1(reYAML, text)
	case fmtPyproject:
		return pyprojectVersion(text)
	case fmtGoConst:
		return cap1(reGoConst, text)
	case fmtNsis:
		return cap1(reNsis, text)
	case fmtInfoJSON:
		// Wails v3 escribe `fixed.file_version` y `info.<lang>.ProductVersion`;
		// `product_version` solo aparece en perfiles antiguos. Basta con uno de
		// los tres para considerar el archivo versionable.
		if v, ok := cap1(reIJFile, text); ok {
			return v, true
		}
		if v, ok := cap1(reIJProduct, text); ok {
			return v, true
		}
		return cap1(reIJProductCap, text)
	case fmtGradle:
		return gradleName(text)
	case fmtGradleProject:
		return cap1(reGradleProjVer, text)
	case fmtPlist:
		return cap1(rePlistFirst, text)
	case fmtPubspec:
		return pubspecVersion(text)
	case fmtWinres:
		return winresVersion(text)
	case fmtRunnerRC:
		return cap1(reRCStrVal, text)
	}
	return "", false
}

// isInsertableFormat indica si sabemos anadir el campo version a un archivo
// que aun no lo tiene sin correr el riesgo de dejarlo invalido.
func isInsertableFormat(f versionFormat) bool {
	return f == fmtJSON || f == fmtTOML || f == fmtGoConst
}

// pubspecVersion devuelve "X.Y.Z+B" (o "X.Y.Z" si no hay build number).
func pubspecVersion(text string) (string, bool) {
	m := rePubspec.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	if m[2] != "" {
		return m[1] + "+" + m[2], true
	}
	return m[1], true
}

// winresVersion lee la version de go-winres.json. El formato Windows tiene
// cuatro componentes (1.0.0.0); mostramos los tres primeros.
func winresVersion(text string) (string, bool) {
	// reJSON captura el valor de "version" (identity); reIJFile es fallback
	// para perfiles sin identity. reWinresIdentity es solo de reemplazo: su
	// grupo 1 es el prefijo, no la version.
	v, ok := cap1(reJSON, text)
	if !ok {
		v, ok = cap1(reIJFile, text)
	}
	if !ok {
		return "", false
	}
	return strings.TrimSuffix(v, ".0"), true
}

// pubspecTarget calcula la version final de Flutter. Si el usuario no da un
// build number explicito (+B), se incrementa el actual (practica habitual en
// Flutter: cada release sube el build).
func pubspecTarget(newVer, current string) (string, error) {
	semver := newVer
	explicitBuild := ""
	if i := strings.IndexByte(newVer, '+'); i >= 0 {
		semver, explicitBuild = newVer[:i], newVer[i+1:]
	}
	if i := strings.IndexByte(semver, '-'); i >= 0 {
		return "", fmt.Errorf("pubspec.yaml no admite sufijos -pre: usa solo X.Y.Z o X.Y.Z+B (B numerico)")
	}
	if explicitBuild != "" {
		if _, err := strconv.ParseUint(explicitBuild, 10, 64); err != nil {
			return "", fmt.Errorf("en Flutter el build number debe ser numerico (p. ej. 1.2.3+5)")
		}
		return semver + "+" + explicitBuild, nil
	}
	var build uint64
	if i := strings.IndexByte(current, '+'); i >= 0 {
		if n, err := strconv.ParseUint(current[i+1:], 10, 64); err == nil {
			build = n
		}
	}
	return fmt.Sprintf("%s+%d", semver, build+1), nil
}

func cap1(re *regexp.Regexp, text string) (string, bool) {
	m := re.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	return m[1], true
}

func gradleName(text string) (string, bool) { return cap1(reGradleName, text) }

func gradleCode(text string) (uint64, bool) {
	m := reGradleCode.FindStringSubmatch(text)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseUint(m[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func missingHint(f versionFormat) string {
	switch f {
	case fmtYAML:
		return `se esperaba "version: \"x.y.z\"" bajo "info:" (ver build/config.yml)`
	case fmtJSON:
		return "se esperaba una propiedad `version` en el objeto raíz"
	case fmtTOML:
		return `se esperaba "version = \"x.y.z\""`
	case fmtGoConst:
		return `se esperaba "const version = \"x.y.z\"" (p. ej. main.go)`
	case fmtNsis:
		return `se esperaba "!define INFO_PRODUCTVERSION \"x.y.z\"" (project.nsi)`
	case fmtInfoJSON:
		return `se esperaban "file_version", "product_version" y "ProductVersion" (info.json)`
	case fmtGradle:
		return `se esperaba "versionName \"x.y.z\"" (build.gradle)`
	case fmtGradleProject:
		return `se esperaba "version = \"x.y.z\"" en build.gradle(.kts)`
	case fmtPlist:
		return `se esperaban "CFBundleShortVersionString" y "CFBundleVersion" (Info.plist)`
	case fmtPyproject:
		return "se esperaba `version` bajo [project] o [tool.poetry]"
	case fmtPubspec:
		return `se esperaba "version: x.y.z+b" en la raiz de pubspec.yaml`
	case fmtWinres:
		return `se esperaban los campos de version en go-winres.json`
	case fmtRunnerRC:
		return `se esperaban VERSION_AS_STRING / VERSION_AS_NUMBER en windows/runner/Runner.rc`
	}
	return ""
}

// windowsVersion separa la parte numerica de X.Y.Z (ignorando sufijos) para
// los formatos que exigen cuatro componentes (go-winres, Runner.rc).
func windowsVersion(newVer string) (dotted, comma string) {
	s := strings.SplitN(newVer, "-", 2)[0]
	s = strings.SplitN(s, "+", 2)[0]
	parts := strings.Split(s, ".")
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	major := parts[0]
	if major == "" {
		major = "0"
	}
	dotted = major + "." + parts[1] + "." + parts[2]
	comma = major + "," + parts[1] + "," + parts[2] + ",0"
	return
}

func applyFormat(f *versionFile, newVer string) (string, error) {
	switch f.format {
	case fmtJSON:
		return reJSON.ReplaceAllString(f.text, fmt.Sprintf(`"version": "%s"`, newVer)), nil
	case fmtTOML:
		return reTOML.ReplaceAllString(f.text, fmt.Sprintf(`version = "%s"`, newVer)), nil
	case fmtYAML:
		return reYAMLReplace.ReplaceAllString(f.text, fmt.Sprintf(`${1}version: "%s"`, newVer)), nil
	case fmtPyproject:
		return replacePyprojectVersion(f.text, newVer), nil
	case fmtGoConst:
		return reGoConstReplace.ReplaceAllString(f.text, fmt.Sprintf(`${1}%s${2}`, newVer)), nil
	case fmtNsis:
		return reNsisReplace.ReplaceAllString(f.text, fmt.Sprintf(`${1}%s${2}`, newVer)), nil
	case fmtInfoJSON:
		first := reIJFile.ReplaceAllString(f.text, fmt.Sprintf(`"file_version": "%s.0"`, newVer))
		second := reIJProduct.ReplaceAllString(first, fmt.Sprintf(`"product_version": "%s.0"`, newVer))
		return reIJProductCap.ReplaceAllString(second, fmt.Sprintf(`"ProductVersion": "%s"`, newVer)), nil
	case fmtGradle:
		renamed := reGradleNameRep.ReplaceAllString(f.text, fmt.Sprintf(`${1}%s${2}`, newVer))
		return reGradleCodeRep.ReplaceAllString(renamed, fmt.Sprintf(`${1}%d`, f.currentCode+1)), nil
	case fmtGradleProject:
		return reGradleProjVerRep.ReplaceAllString(f.text, fmt.Sprintf(`${1}%s${2}`, newVer)), nil
	case fmtPlist:
		return rePlist.ReplaceAllString(f.text, fmt.Sprintf(`${1}%s${2}`, newVer)), nil
	case fmtPubspec:
		target, err := pubspecTarget(newVer, f.current)
		if err != nil {
			return "", err
		}
		return rePubspecReplace.ReplaceAllString(f.text, fmt.Sprintf(`${1}%s${3}`, target)), nil
	case fmtWinres:
		dotted, _ := windowsVersion(newVer)
		v4 := dotted + ".0"
		// Los tres regex capturan la clave (grupo 1), asi que se conserva tal
		// cual: file_version y product_version no deben confundirse.
		text := reWinresIdentity.ReplaceAllString(f.text, fmt.Sprintf(`${1}%s${2}`, v4))
		text = reWinresFixed.ReplaceAllString(text, fmt.Sprintf(`${1}%s${2}`, v4))
		return reWinresInfo.ReplaceAllString(text, fmt.Sprintf(`${1}%s${2}`, v4)), nil
	case fmtRunnerRC:
		dotted, comma := windowsVersion(newVer)
		text := reRCStr.ReplaceAllString(f.text, fmt.Sprintf(`${1}%s${2}`, dotted))
		return reRCNum.ReplaceAllString(text, fmt.Sprintf(`${1}%s`, comma)), nil
	}
	return f.text, nil
}

func tomlSectionRange(text, name string) (int, int, bool) {
	offset := 0
	start := -1
	lines := strings.SplitAfter(text, "\n")
	pos := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if start < 0 {
			if trimmed == name {
				start = pos + len(line)
			}
		} else if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			return start, pos, true
		}
		pos += len(line)
		offset += len(line)
	}
	if start >= 0 {
		return start, len(text), true
	}
	return 0, 0, false
}

func pyprojectVersion(text string) (string, bool) {
	for _, section := range []string{"[project]", "[tool.poetry]"} {
		s, e, ok := tomlSectionRange(text, section)
		if !ok {
			continue
		}
		if m := reTOML.FindStringSubmatch(text[s:e]); m != nil {
			return m[1], true
		}
	}
	return "", false
}

func replacePyprojectVersion(text, version string) string {
	for _, name := range []string{"[project]", "[tool.poetry]"} {
		s, e, ok := tomlSectionRange(text, name)
		if !ok {
			continue
		}
		section := text[s:e]
		if rePyprojectVer.MatchString(section) {
			updated := rePyprojectVer.ReplaceAllString(section, fmt.Sprintf(`${1}%s${2}`, version))
			return text[:s] + updated + text[e:]
		}
	}
	return text
}

func jsonIsObject(text string) bool {
	t := strings.TrimSpace(text)
	return strings.HasPrefix(t, "{")
}

func projectTypeLabel(pt project.ProjectType) string {
	switch pt {
	case project.TypeGo:
		return "GO"
	case project.TypeGoWails:
		return "GO + WAILS"
	case project.TypePython:
		return "PYTHON"
	default:
		return pt.Label()
	}
}
