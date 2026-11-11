// Package window localiza la ventana de otra instancia de Compass que ya esta
// abierta, para traerla al frente en vez de abrir una segunda.
package window

import "strings"

// titleMatches indica si el titulo de una ventana corresponde al del proyecto.
//
// Las consolas elevadas anteponen un prefijo localizado al titulo
// ("Administrator: ", "Administrador: "), asi que tambien vale cuando el titulo
// termina en ": <titulo>".
func titleMatches(have, want string) bool {
	have = strings.TrimSpace(have)
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	if strings.EqualFold(have, want) {
		return true
	}
	if i := strings.LastIndex(have, ": "); i >= 0 {
		return strings.EqualFold(strings.TrimSpace(have[i+2:]), want)
	}
	return false
}
