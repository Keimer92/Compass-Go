package tasks

import (
	"path/filepath"
	"strings"
)

// isProjectWhatsappEnginePath decide si la ruta completa de un node.exe
// pertenece al motor de WhatsApp de ESTE proyecto. Solo se cierran los motores
// que viven bajo la carpeta del proyecto; nunca los del sistema (C:\Program
// Files\nodejs) ni los de otros proyectos, aunque también sean node.exe.
//
// Se consideran motor las rutas bajo <raiz>/whatsapp_engine/ (staging y release
// de Flutter) y <raiz>/resources/whatsapp-engine-bundle/ (Go/Wails).
func isProjectWhatsappEnginePath(imagePath, projectRoot string) bool {
	root := strings.ToLower(filepath.ToSlash(projectRoot))
	p := strings.ToLower(filepath.ToSlash(imagePath))
	root = strings.TrimRight(root, "/\\")
	if root == "" || !strings.HasPrefix(p, root+"/") {
		return false
	}
	rel := strings.TrimPrefix(p, root+"/")
	return strings.HasPrefix(rel, "whatsapp_engine/") ||
		strings.HasPrefix(rel, "build/") && strings.Contains(rel, "/whatsapp_engine/") ||
		strings.HasPrefix(rel, "resources/whatsapp-engine-bundle/")
}
