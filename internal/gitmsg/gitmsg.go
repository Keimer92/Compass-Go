// Package gitmsg traduce errores frecuentes de git a mensajes accionables
// para el usuario, en lugar del volcado crudo del comando.
package gitmsg

import (
	"fmt"
	"regexp"
	"strings"
)

// `remote: Permission to Keimer92/Compass-Go.git denied to Keimer92-C5.`
// Los usuarios de GitHub solo llevan letras, digitos y guiones.
var deniedRe = regexp.MustCompile(`(?i)permission to ([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)\.git denied to ([A-Za-z0-9-]+)`)

// `fatal: unable to access '...': The requested URL returned error: 403`
var forbiddenRe = regexp.MustCompile(`(?i)returned error: 403`)

// Denied describe el rechazo de GitHub por permisos: que cuenta se autentico
// y sobre que repositorio no tiene escritura.
type Denied struct {
	Repo    string // owner/repo
	Owner   string
	Account string
}

// ParseDenied reconoce `remote: Permission to <repo>.git denied to <cuenta>`.
func ParseDenied(output string) (Denied, bool) {
	m := deniedRe.FindStringSubmatch(output)
	if m == nil {
		return Denied{}, false
	}
	d := Denied{Repo: m[1], Owner: m[1], Account: m[2]}
	if i := strings.IndexByte(d.Repo, '/'); i > 0 {
		d.Owner = d.Repo[:i]
	}
	return d, true
}

// IsForbidden detecta el 403 generico de GitHub, que tambien aparece cuando no
// se pudo identificar la cuenta.
func IsForbidden(output string) bool { return forbiddenRe.MatchString(output) }

// ExplainPush devuelve un mensaje con la causa y los comandos para arreglarla
// cuando `git push` falla por permisos. Devuelve "" si no reconoce el error.
//
// Es la version para consola; la GUI manda al usuario al modal de ajustes, que
// tiene las mismas acciones como botones.
//
// El caso tipico en Windows: Git Credential Manager tiene guardada otra cuenta
// de GitHub y autentica con ella, asi que GitHub responde 403 aunque el repo
// sea del usuario. Compass no elige la cuenta: solo ejecuta `git push`.
func ExplainPush(output string) string {
	if d, ok := ParseDenied(output); ok {
		return fmt.Sprintf(`GitHub autenticó como "%s", que no tiene permiso de escritura sobre "%s".

Esa cuenta la aporta el credential helper de Windows (Git Credential Manager), no Compass. Para usar otra:

  git credential-manager github login --username %s
  git credential-manager github logout %s

Si el repositorio pertenece a una organización, pon tu usuario de GitHub en --username.`,
			d.Account, d.Repo, d.Owner, d.Account)
	}
	if IsForbidden(output) {
		return `GitHub rechazó el push con 403: la credencial guardada no tiene permiso de escritura sobre este repositorio.

Esa credencial la aporta el credential helper de Windows (Git Credential Manager), no Compass. Comprueba la cuenta y vuelve a iniciar sesión:

  git credential-manager github list
  git credential-manager github login --username TU_USUARIO

Si tu cuenta sí tiene acceso, puede faltar autorizar el token para el SSO de la organización.`
	}
	return ""
}
