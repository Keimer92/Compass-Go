package tasks

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"compass/internal/project"
	"compass/internal/prompts"
)

// Device es un dispositivo Android físico conectado.
type Device struct {
	Serial string
}

func (d Device) label() string { return "Físico: " + d.Serial }

// adbCommand ejecuta adb con los argumentos dados en dir (si es "" usa el cwd).
func adbCommand(ctx *project.ProjectContext, args ...string) (string, error) {
	adb := ctx.AdbPath
	if adb == "" {
		adb = "adb"
	}
	out, err := exec.Command(adb, args...).CombinedOutput()
	return string(out), err
}

// listPhysicalDevices lista los dispositivos físicos conectados (estado
// "device", sin emuladores). Es fiel a utils/android.rs::list_physical_devices.
func listPhysicalDevices(ctx *project.ProjectContext) ([]Device, error) {
	out, err := adbCommand(ctx, "devices", "-l")
	if err != nil {
		return nil, fmt.Errorf("[ERROR] No se pudo ejecutar adb.\n" +
			"       Verifica que platform-tools esté instalado y que el servidor\n" +
			"       adb funcione: adb kill-server && adb start-server")
	}
	var devices []Device
	for _, line := range strings.Split(out, "\n")[1:] {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		fields := strings.Fields(t)
		serial := fields[0]
		rest := strings.TrimSpace(strings.TrimPrefix(t, serial))
		if !strings.HasPrefix(rest, "device") {
			continue
		}
		if serial == "" || strings.HasPrefix(serial, "emulator-") {
			continue
		}
		devices = append(devices, Device{Serial: serial})
	}
	// dedupe
	seen := map[string]bool{}
	var uniq []Device
	for _, d := range devices {
		if !seen[d.Serial] {
			seen[d.Serial] = true
			uniq = append(uniq, d)
		}
	}
	return uniq, nil
}

// waitForDeviceBoot espera a que el teléfono termine de arrancar
// (sys.boot_completed == 1). Equivalente de utils/android.rs::wait_for_device_boot.
func waitForDeviceBoot(ctx *project.ProjectContext, serial string) error {
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		out, err := adbCommand(ctx, "-s", serial, "shell", "getprop", "sys.boot_completed")
		if err == nil && strings.TrimSpace(out) == "1" {
			fmt.Printf("[OK] Dispositivo listo: %s\n", serial)
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("[ERROR] Timeout esperando a que el dispositivo %s termine de arrancar", serial)
}

// selectAndroidDevice elige el dispositivo físico: auto-selecciona si hay uno,
// pregunta si hay varios, o da error accionable si no hay ninguno.
func selectAndroidDevice(ctx *project.ProjectContext) (string, error) {
	if ctx.AdbPath == "" {
		return "", fmt.Errorf("[ERROR] No se encontró adb.\n" +
			"       Instala platform-tools y define ANDROID_HOME:\n" +
			"         sdkmanager \"platform-tools\"\n" +
			"       Y revisa que el teléfono tenga depuración USB activada.")
	}
	phys, err := listPhysicalDevices(ctx)
	if err != nil {
		return "", err
	}
	switch len(phys) {
	case 0:
		return "", fmt.Errorf("[ERROR] No se detectó ningún dispositivo Android.\n" +
			"\n" +
			"  Para conectarlo:\n" +
			"    1. Activa 'Depuración USB' en Opciones de desarrollador del teléfono\n" +
			"    2. Conéctalo por cable (o usa depuración por Wi-Fi)\n" +
			"    3. Acepta la huella RSA que aparece en pantalla\n" +
			"    4. Desbloquea la pantalla: el dispositivo sólo aparece cuando\n" +
			"       está desbloqueado y en modo depuración\n\n" +
			"  Verifica con: adb devices -l")
	case 1:
		fmt.Printf("[OK] Dispositivo físico detectado: %s\n", phys[0].Serial)
		if err := waitForDeviceBoot(ctx, phys[0].Serial); err != nil {
			return "", err
		}
		return phys[0].Serial, nil
	default:
		labels := make([]string, len(phys))
		for i, d := range phys {
			labels[i] = d.label()
		}
		idx, err := prompts.Select("Varios dispositivos. Selecciona uno:", labels)
		if err != nil {
			return "", err
		}
		serial := phys[idx].Serial
		if err := waitForDeviceBoot(ctx, serial); err != nil {
			return "", err
		}
		return serial, nil
	}
}

// installApk instala un APK ya compilado en el dispositivo y abre la app.
// Equivalente de utils/android.rs::install_apk (maneja choque de firmas).
func installApk(ctx *project.ProjectContext, apk, pkg, activity string) error {
	serial, err := selectAndroidDevice(ctx)
	if err != nil {
		return err
	}
	combined, err := adbCommand(ctx, "-s", serial, "install", "-r", apk)
	if err != nil {
		return fmt.Errorf("adb install falló: %w", err)
	}
	installed := strings.Contains(combined, "Success")
	if !installed && (strings.Contains(combined, "INSTALL_FAILED_UPDATE_INCOMPATIBLE") ||
		strings.Contains(combined, "signatures do not match") ||
		strings.Contains(combined, "INSTALL_FAILED_VERSION_DOWNGRADE")) {
		fmt.Println("[WARN] La app ya instalada esta firmada con otra clave (debug).")
		if pkg != "" {
			ok, _ := prompts.Confirm("¿Desinstalar la version anterior e instalar la release?", true)
			if ok {
				_, _ = adbCommand(ctx, "-s", serial, "uninstall", pkg)
				combined, _ = adbCommand(ctx, "-s", serial, "install", "-r", apk)
				installed = strings.Contains(combined, "Success")
				if !installed {
					fmt.Println("[ERROR] La instalacion fallo:")
					fmt.Println(strings.TrimSpace(combined))
				}
			}
		}
	} else if !installed {
		fmt.Println("[ERROR] La instalacion fallo:")
		fmt.Println(strings.TrimSpace(combined))
	}

	if installed {
		fmt.Printf("[OK] APK instalado en %s\n", serial)
		if pkg != "" && activity != "" {
			component := pkg + "/" + activity
			_, _ = adbCommand(ctx, "-s", serial, "shell", "am", "start", "-n", component)
			fmt.Println("[OK] Abriendo aplicacion...")
		}
	}
	return nil
}

// installApkIfDevice instala el APK sólo si hay un dispositivo conectado.
// Nunca aborta el build: si falla, avisa y sigue. Equivalente de
// utils/android.rs::install_apk_if_device.
func installApkIfDevice(ctx *project.ProjectContext, apk, pkg, activity string) {
	devices, err := listPhysicalDevices(ctx)
	if err != nil || len(devices) == 0 {
		fmt.Println()
		fmt.Println("[INFO] No hay ningun dispositivo Android conectado: no se instala.")
		fmt.Printf("[OK] APK disponible en: %s\n", apk)
		return
	}
	fmt.Printf("[INFO] Dispositivo Android detectado: %s\n", devices[0].Serial)
	if err := installApk(ctx, apk, pkg, activity); err != nil {
		fmt.Println()
		fmt.Printf("[WARN] No se pudo instalar en el dispositivo:\n       %s\n", err)
	}
	fmt.Printf("[OK] APK disponible en: %s\n", apk)
}

// ensureGradle comprueba que exista gradlew.bat en la raíz del proyecto.
func ensureGradle(ctx *project.ProjectContext) error {
	gradlew := filepath.Join(ctx.ScriptDir, "gradlew.bat")
	if !fileExists(gradlew) {
		return fmt.Errorf("No se encontró el wrapper de Gradle en la raíz del proyecto.\n"+
			"   Se esperaba `%s`.", gradlew)
	}
	return nil
}
