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

// ExplainPush devuelve un mensaje con la causa y los comandos para arreglarla
// cuando `git push` falla por permisos. Devuelve "" si no reconoce el error.
//
// El caso tipico en Windows: Git Credential Manager tiene guardada otra cuenta
// de GitHub y autentica con ella, asi que GitHub responde 403 aunque el repo
// sea del usuario. Compass no elige la cuenta: solo ejecuta `git push`.
func ExplainPush(output string) string {
	if m := deniedRe.FindStringSubmatch(output); m != nil {
		repo, account := m[1], m[2]
		owner := repo
		if i := strings.IndexByte(repo, '/'); i > 0 {
			owner = repo[:i]
		}
		return fmt.Sprintf(`GitHub autenticó como "%s", que no tiene permiso de escritura sobre "%s".

Esa cuenta la aporta el credential helper de Windows (Git Credential Manager), no Compass. Para usar otra:

  git credential-manager github login --username %s
  git credential-manager github logout %s

Si el repositorio pertenece a una organización, pon tu usuario de GitHub en --username.`,
			account, repo, owner, account)
	}
	if forbiddenRe.MatchString(output) {
		return `GitHub rechazó el push con 403: la credencial guardada no tiene permiso de escritura sobre este repositorio.

Esa credencial la aporta el credential helper de Windows (Git Credential Manager), no Compass. Comprueba la cuenta y vuelve a iniciar sesión:

  git credential-manager github list
  git credential-manager github login --username TU_USUARIO

Si tu cuenta sí tiene acceso, puede faltar autorizar el token para el SSO de la organización.`
	}
	return ""
}
