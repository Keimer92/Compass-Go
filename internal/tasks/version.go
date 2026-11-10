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
	Register("version", project.TypeTauri, VersionBump)
	Register("version", project.TypeFlutter, VersionBump)
	Register("version", project.TypeKotlin, VersionBump)
	Register("version", project.TypePython, VersionBump)
	Register("version", project.TypeRust, VersionBump)
	Register("version", project.TypeGo, VersionBump)
	Register("version", project.TypeGoWails, VersionBump)
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
	reGradleName     = regexp.MustCompile(`(?m)^[ \t]*versionName[ \t]+"([^"]+)"`)
	reGradleCode     = regexp.MustCompile(`(?m)^[ \t]*versionCode[ \t]+(\d+)`)
	reGradleNameRep  = regexp.MustCompile(`(?m)^([ \t]*versionName[ \t]+")[^"]+(")`)
	reGradleCodeRep  = regexp.MustCompile(`(?m)^([ \t]*versionCode[ \t]+)(\d+)`)
	rePyprojectVer   = regexp.MustCompile(`(?m)^([ \t]*version[ \t]*=[ \t]*")[^"]+(".*)$`)
	rePackage        = regexp.MustCompile(`(?m)^\[package\]\s*$`)
	reNewVersion     = regexp.MustCompile(`^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$`)
)

// VersionBump actualiza la version del proyecto (port de versionar.bat).
// Equivalente de tasks/version.rs::bump.
func VersionBump(ctx *project.ProjectContext) error {
	shell.ClearScreen()
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║              VERSIONADOR DE PROYECTO              ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()

	if ctx.ProjectType == project.TypeFlutter || ctx.ProjectType == project.TypeKotlin ||
		ctx.ProjectType == project.TypeGo {
		fmt.Println("[INFO] No aplicable: la version se gestiona en la configuracion propia del proyecto.")
		fmt.Println()
		ui.Pause()
		return nil
	}

	configs := versionConfigs(ctx)

	// Fase de VALIDACION (todo-o-nada).
	var files []versionFile
	var omitted []string
	for _, sf := range configs {
		if !fileExists(sf.path) {
			if sf.source == srcRequired {
				return fmt.Errorf("no existe el archivo: %s", sf.path)
			}
			omitted = append(omitted, sf.path)
			continue
		}
		data, err := os.ReadFile(sf.path)
		if err != nil {
			return fmt.Errorf("no se pudo leer %s: %w", sf.path, err)
		}
		text := strings.TrimPrefix(string(data), "\ufeff")

		var cur string
		var hasCur bool
		if sf.format == fmtGradle {
			cc, ok := gradleCode(text)
			if ok {
				cur, hasCur = gradleName(text)
			} else {
				cur, hasCur = "", false
			}
			if hasCur {
				if !ok {
					return fmt.Errorf("no se encontro `versionCode` en: %s\n  Anadelo dentro del bloque defaultConfig.", sf.path)
				}
				files = append(files, versionFile{path: sf.path, format: sf.format, current: cur, hasCurrent: true, currentCode: cc, hasCode: true, text: text})
				continue
			}
		} else {
			cur, hasCur = detectVersion(sf.format, text)
		}

		if hasCur {
			files = append(files, versionFile{path: sf.path, format: sf.format, current: cur, hasCurrent: true, text: text})
			continue
		}

		if sf.format == fmtPyproject {
			return fmt.Errorf("no se encontró una versión estática dentro de `[project]` o `[tool.poetry]` en %s. Los proyectos con version dinámica deben actualizarla en su sistema de build.", sf.path)
		}
		if sf.format == fmtYAML {
			return fmt.Errorf("no se encontro el campo `version:` en: %s\n  Añádelo bajo `info:` y vuelve a intentarlo.", sf.path)
		}
		if sf.format != fmtJSON && sf.format != fmtTOML {
			return fmt.Errorf("no se encontro el marcador de version en: %s\n  %s", sf.path, missingHint(sf.format))
		}
		if sf.format == fmtTOML {
			if !rePackage.MatchString(text) {
				return fmt.Errorf("Cargo.toml sin version y sin seccion [package]:\n  no se puede insertar de forma segura: %s", sf.path)
			}
		} else {
			if !strings.HasPrefix(strings.TrimSpace(text), "{") {
				// Validación ligera de objeto JSON raíz.
				if !jsonIsObject(text) {
					return fmt.Errorf("JSON no es un objeto; no se puede insertar version de forma segura: %s", sf.path)
				}
			}
		}
		files = append(files, versionFile{path: sf.path, format: sf.format, hasCurrent: false, text: text})
	}

	fmt.Printf("  Tipo de proyecto: %s\n", projectTypeLabel(ctx.ProjectType))
	fmt.Println()
	for _, o := range omitted {
		rel, _ := filepath.Rel(ctx.ScriptDir, o)
		fmt.Printf("  (no existe, se omite)   %s\n", rel)
	}
	for _, f := range files {
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
	if !reNewVersion.MatchString(newVer) {
		return fmt.Errorf("formato invalido. Usa X.Y.Z (ej: 1.2.3) o con sufijo (ej: 2.0.0-beta.1)")
	}

	var added int
	var gradleNotes []string
	for i := range files {
		f := &files[i]
		var newText string
		if f.hasCurrent {
			if f.format == fmtGradle {
				gradleNotes = append(gradleNotes, fmt.Sprintf("%s -> %d", f.path, f.currentCode+1))
			}
			newText = applyFormat(f, newVer)
		} else {
			added++
			if f.format == fmtTOML {
				newText = rePackage.ReplaceAllString(f.text, fmt.Sprintf("[package]\nversion = \"%s\"", newVer))
			} else {
				idx := strings.Index(f.text, "{")
				if idx < 0 {
					return fmt.Errorf("no se encontro el inicio del objeto JSON: %s", f.path)
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
			return fmt.Errorf("no se pudo escribir %s: %w", f.path, err)
		}
	}

	fmt.Printf("[OK] Version actualizada a %s en %d archivo(s).\n", newVer, len(files))
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

func versionConfigs(ctx *project.ProjectContext) []sourceFile {
	root := ctx.ScriptDir
	mk := func(rel string, f versionFormat, s versionSource) sourceFile {
		return sourceFile{path: filepath.Join(root, rel), format: f, source: s}
	}
	switch ctx.ProjectType {
	case project.TypeTauri:
		return []sourceFile{
			mk("package.json", fmtJSON, srcRequired),
			mk("src-tauri/tauri.conf.json", fmtJSON, srcRequired),
			mk("src-tauri/Cargo.toml", fmtTOML, srcRequired),
		}
	case project.TypeRust:
		return []sourceFile{mk("Cargo.toml", fmtTOML, srcRequired)}
	case project.TypeGoWails:
		return []sourceFile{
			mk("build/config.yml", fmtYAML, srcRequired),
			mk("main.go", fmtGoConst, srcOptional),
			mk("frontend/package.json", fmtJSON, srcOptional),
			mk("build/windows/nsis/project.nsi", fmtNsis, srcOptional),
			mk("build/windows/info.json", fmtInfoJSON, srcOptional),
			mk("build/android/app/build.gradle", fmtGradle, srcOptional),
			mk("build/android/custom/app/build.gradle", fmtGradle, srcOptional),
			mk("build/ios/Info.plist", fmtPlist, srcOptional),
			mk("build/ios/Info.dev.plist", fmtPlist, srcOptional),
			mk("build/darwin/Info.plist", fmtPlist, srcOptional),
			mk("build/darwin/Info.dev.plist", fmtPlist, srcOptional),
		}
	case project.TypePython:
		return []sourceFile{mk("pyproject.toml", fmtPyproject, srcRequired)}
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
		fv, ok1 := cap1(reIJFile, text)
		_, ok2 := cap1(reIJProduct, text)
		_, ok3 := cap1(reIJProductCap, text)
		if ok1 && ok2 && ok3 {
			return fv, true
		}
		return "", false
	case fmtGradle:
		return gradleName(text)
	case fmtPlist:
		return cap1(rePlistFirst, text)
	}
	return "", false
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
	case fmtPlist:
		return `se esperaban "CFBundleShortVersionString" y "CFBundleVersion" (Info.plist)`
	case fmtPyproject:
		return "se esperaba `version` bajo [project] o [tool.poetry]"
	}
	return ""
}

func applyFormat(f *versionFile, newVer string) string {
	switch f.format {
	case fmtJSON:
		return reJSON.ReplaceAllString(f.text, fmt.Sprintf(`"version": "%s"`, newVer))
	case fmtTOML:
		return reTOML.ReplaceAllString(f.text, fmt.Sprintf(`version = "%s"`, newVer))
	case fmtYAML:
		return reYAMLReplace.ReplaceAllString(f.text, fmt.Sprintf(`${1}version: "%s"`, newVer))
	case fmtPyproject:
		return replacePyprojectVersion(f.text, newVer)
	case fmtGoConst:
		return reGoConstReplace.ReplaceAllString(f.text, fmt.Sprintf(`${1}%s${2}`, newVer))
	case fmtNsis:
		return reNsisReplace.ReplaceAllString(f.text, fmt.Sprintf(`${1}%s${2}`, newVer))
	case fmtInfoJSON:
		first := reIJFile.ReplaceAllString(f.text, fmt.Sprintf(`"file_version": "%s.0"`, newVer))
		second := reIJProduct.ReplaceAllString(first, fmt.Sprintf(`"product_version": "%s.0"`, newVer))
		return reIJProductCap.ReplaceAllString(second, fmt.Sprintf(`"ProductVersion": "%s"`, newVer))
	case fmtGradle:
		renamed := reGradleNameRep.ReplaceAllString(f.text, fmt.Sprintf(`${1}%s${2}`, newVer))
		return reGradleCodeRep.ReplaceAllString(renamed, fmt.Sprintf(`${1}%d`, f.currentCode+1))
	case fmtPlist:
		return rePlist.ReplaceAllString(f.text, fmt.Sprintf(`${1}%s${2}`, newVer))
	}
	return f.text
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
	case project.TypeTauri:
		return "TAURI"
	case project.TypeGo:
		return "GO"
	case project.TypeGoWails:
		return "GO + WAILS"
	case project.TypePython:
		return "PYTHON"
	default:
		return "RUST"
	}
}
