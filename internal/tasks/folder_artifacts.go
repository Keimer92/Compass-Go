package tasks

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"compass/internal/project"
	"compass/internal/shell"
)

//go:embed assets/portable-launcher.go.txt
var folderPortableLauncher string

//go:embed assets/app-folder-installer.nsi
var folderInstallerTemplate string

// hostArchLabel devuelve la etiqueta de arquitectura del equipo, la unica que
// puede compilar Flutter para escritorio (no acepta --target-platform).
func HostArchLabel() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "386":
		return "x86"
	case "arm64":
		return "ARM64"
	}
	return runtime.GOARCH
}

// writeFolderArchive empaqueta srcDir en un .tar.gz con la maxima compresion.
// Se usa tar+gzip (stdlib) para que el stub portable pueda extraerlo sin
// dependencias externas. exeName (si se da) se renombra a app.exe dentro del
// archive para que el stub pueda lanzarlo con un nombre fijo y sin espacios.
func writeFolderArchive(srcDir, exeName, outPath string) error {
	file, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer file.Close()

	gz, err := gzip.NewWriterLevel(file, gzip.BestCompression)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(gz)

	walkErr := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		// Renombra el exe real a app.exe: asi el stub lo lanza siempre con un
		// nombre fijo y sin espacios (los ldflags -X no toleran espacios).
		if exeName != "" && strings.EqualFold(rel, exeName) {
			hdr.Name = "app.exe"
		}
		// Metadatos irrelevantes en Windows y que rompen la reproducibilidad.
		hdr.Uid, hdr.Gid = 0, 0
		hdr.Uname, hdr.Gname = "", ""
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, in)
		closeErr := in.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if walkErr != nil {
		return walkErr
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// fileHash devuelve el SHA-256 de un archivo.
func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// buildFolderPortable envuelve una carpeta de release completa en un unico .exe
// que se autoextrae al arrancar. exeName es el nombre del ejecutable real dentro
// del release (p. ej. school_tracker_pro_ela.exe). Devuelve la ruta del portable.
func buildFolderPortable(ctx *project.ProjectContext, releaseDir, exeName, appName, version string) (string, error) {
	if !dirExists(releaseDir) {
		return "", fmt.Errorf("no se encontro la carpeta del release: %s", releaseDir)
	}
	if appName == "" {
		appName = sanitizeAppName(filepath.Base(ctx.ScriptDir))
	}
	if appName == "" {
		appName = "App"
	}
	safeName := sanitizeAppName(appName)
	if safeName == "" {
		safeName = "CompassApp"
	}

	tmp, err := os.MkdirTemp("", "compass-folder-portable-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	archive := filepath.Join(tmp, "payload.tar.gz")
	fmt.Println("[INFO] Comprimiendo el release (puede tardar unos minutos)...")
	if err := writeFolderArchive(releaseDir, exeName, archive); err != nil {
		return "", fmt.Errorf("no se pudo comprimir el release: %w", err)
	}
	info, err := os.Stat(archive)
	if err != nil {
		return "", err
	}
	hash, err := fileHash(archive)
	if err != nil {
		return "", err
	}
	buildID := hash[:16]

	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module compassportable\n\ngo 1.22\n"), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(tmp, "main.go"), []byte(folderPortableLauncher), 0o644); err != nil {
		return "", err
	}
	writeStubIconResource(tmp, findProjectIcon(ctx), runtime.GOARCH)

	outDir := filepath.Join(ctx.ScriptDir, "bin")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	outPath := filepath.Join(outDir, artifactBase(appName, version, HostArchLabel()+" portable")+".exe")

	// El exe se renombro a app.exe dentro del archive, asi que el nombre del
	// stub es fijo y no depende del nombre real del release (que puede tener
	// espacios y romper los ldflags -X).
	ldflags := fmt.Sprintf("-s -w -X main.appName=%s -X main.appExe=app.exe -X main.buildID=%s",
		safeName, buildID)
	fmt.Printf("[INFO] Generando portable de un único exe: %s\n", filepath.Base(outPath))
	fmt.Printf("       payload comprimido: %.1f MB\n", float64(info.Size())/1048576.0)
	ok, err := shell.RunProgram("go", []string{
		"build", "-trimpath", "-ldflags", ldflags, "-o", outPath, ".",
	}, tmp, map[string]string{"CGO_ENABLED": "0"})
	if err != nil {
		return "", err
	}
	if !ok || !fileExists(outPath) {
		return "", fmt.Errorf("`go build` del portable de carpeta fallo")
	}
	fmt.Printf("[OK] Portable generado: %s\n", outPath)
	return outPath, nil
}

// buildFolderInstaller genera un Setup NSIS que empaqueta la carpeta del
// release completa (exe + DLLs + data/ + motor de WhatsApp).
func buildFolderInstaller(ctx *project.ProjectContext, releaseDir, exeName, appName, version string) (string, error) {
	if !dirExists(releaseDir) {
		return "", fmt.Errorf("no se encontro la carpeta del release: %s", releaseDir)
	}
	if appName == "" {
		appName = sanitizeAppName(filepath.Base(ctx.ScriptDir))
	}
	if appName == "" {
		appName = "App"
	}
	regKey := sanitizeAppName(appName)
	if regKey == "" {
		regKey = "App"
	}
	makensis := locateMakensis()
	if makensis == "" {
		return "", fmt.Errorf("NSIS (`makensis`) no encontrado; instala NSIS para generar el instalador")
	}

	outDir := filepath.Join(ctx.ScriptDir, "bin")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	outFile := filepath.Join(outDir, artifactBase(appName, version, HostArchLabel())+" Setup.exe")

	script := strings.NewReplacer(
		"@APPNAME@", nsisValue(appName),
		"@APPEXE@", nsisValue(exeName),
		"@APPVER@", nsisValue(version),
		"@APPICON@", nsisValue(findProjectIcon(ctx)),
		"@PAYLOADDIR@", nsisValue(releaseDir),
		"@OUTFILE@", nsisValue(outFile),
		"@REGKEY@", nsisValue(regKey),
	).Replace(folderInstallerTemplate)

	tmp, err := os.CreateTemp("", "compass-folder-*.nsi")
	if err != nil {
		return "", err
	}
	scriptPath := tmp.Name()
	defer os.Remove(scriptPath)
	if _, err := tmp.WriteString(script); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}

	env := map[string]string{}
	if makensis != "makensis" {
		env = nsisEnv(filepath.Dir(makensis))
	}
	fmt.Printf("\n[INFO] Generando instalador: %s\n", filepath.Base(outFile))
	ok, err := shell.RunProgram(makensis, []string{"/V2", scriptPath}, filepath.Dir(scriptPath), env)
	if err != nil {
		return "", err
	}
	if !ok || !fileExists(outFile) {
		return "", fmt.Errorf("makensis no pudo generar el instalador de %s", appName)
	}
	fmt.Printf("[OK] Instalador generado: %s\n", outFile)
	return outFile, nil
}
