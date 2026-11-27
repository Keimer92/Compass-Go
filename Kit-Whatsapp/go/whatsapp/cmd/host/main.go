// whatsapp-host es el sidecar del motor de WhatsApp para apps que no pueden
// enlazar Go (Flutter). Resuelve Node y el bundle, lanza el motor y reenvía
// stdio: la app sigue hablando el mismo protocolo de líneas que con node.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Keimer92/Compass-Go/Kit-Whatsapp/go/whatsapp"
)

func main() {
	app := flag.String("app", "WhatsApp", "nombre de la app (log y mensajes)")
	version := flag.String("version", "dev", "version del build")
	dataDir := flag.String("data-dir", "", "carpeta de datos (log y cwd del motor)")
	authDir := flag.String("auth-dir", "", "carpeta de sesión")
	bundle := flag.String("bundle", "", "ruta de index.js (por defecto se resuelve)")
	flag.Parse()

	whatsapp.Configure(whatsapp.Options{
		AppName:    *app,
		AppVersion: *version,
		DataDir:    *dataDir,
	})
	code := whatsapp.RunHost(whatsapp.HostOptions{
		Bundle:  *bundle,
		AuthDir: *authDir,
		WorkDir: *dataDir,
	})
	if code != 0 {
		fmt.Fprintln(os.Stderr, "[ERROR] whatsapp-host termino con código", code)
	}
	os.Exit(code)
}
