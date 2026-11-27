// Package whatsapp es el cliente del motor de WhatsApp canónico de Compass Go.
//
// Lanza el motor Node (Baileys) como proceso hijo, traduce su protocolo de
// líneas (CONFIG por stdin, eventos por stdout) a callbacks, y gestiona el
// ciclo de vida: resolución de Node, instancia única, sesión, QR, envíos y
// logs. Es agnóstico del proyecto: lo que cambia entre apps (nombre, versión y
// carpeta de datos) se pasa en Options.
package whatsapp

import (
	"os"
	"path/filepath"
	"sync"
)

// Options identifica a la app que usa el supervisor. Es lo único que cambia
// entre proyectos.
type Options struct {
	// AppName aparece en el log, en el nombre del mutex de instancia única y
	// en el mensaje de "otra ventana ya está usando WhatsApp".
	AppName string
	// AppVersion se escribe en la cabecera del log de diagnóstico.
	AppVersion string
	// Profile identifica un perfil de desarrollo (p. ej. una BD aparte). Solo
	// se usa para etiquetar el log; vacío = "default".
	Profile string
	// DataDir es la carpeta de datos de la app: ahí viven whatsapp.log y
	// whatsapp_auth/. Vacío = carpeta del usuario.
	DataDir string
}

var cfg = Options{AppName: "App", AppVersion: "dev"}

// Configure fija las opciones del paquete. Debe llamarse una vez al arrancar,
// antes de New. Vuelve a calcular las rutas de log ya memoizadas.
func Configure(o Options) {
	if o.AppName == "" {
		o.AppName = "App"
	}
	if o.AppVersion == "" {
		o.AppVersion = "dev"
	}
	if o.DataDir == "" {
		o.DataDir = defaultDataDir()
	}
	cfg = o
	logPathOnce = sync.Once{}
	diagPathOnce = sync.Once{}
}

// Options actuales (para tests).
func options() Options { return cfg }

// defaultDataDir cae en la carpeta de datos del usuario cuando la app no dice
// cuál es la suya.
func defaultDataDir() string {
	if v := os.Getenv("APPDATA"); v != "" {
		return filepath.Join(v, "CompassWhatsapp")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".compass-whatsapp")
	}
	return "."
}

// exeDir es el directorio del ejecutable de la app.
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}
