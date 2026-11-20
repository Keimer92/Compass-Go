// Package assets expone los recursos que viajan dentro del binario.
package assets

import _ "embed"

// CompassICO es el icono de la aplicacion (multi-tamano 16..256).
//
// Wails busca en Windows el recurso de icono con ID 3 del .exe; go-winres lo
// embebe con ID 1, asi que la barra de titulo salia sin icono. Pasando estos
// bytes a application.Options.Icon, Wails cae a este icono y la ventana lo
// muestra.
//
//go:embed icon/CompassGo.ico
var CompassICO []byte
