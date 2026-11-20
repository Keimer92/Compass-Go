package webui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// resolveInProject valida que p este dentro de la raiz del proyecto y devuelve
// su ruta absoluta. Evita que el frontend escape del proyecto con ".." o con
// una ruta absoluta arbitraria: las operaciones de archivos solo tocan el
// proyecto activo.
func (b *Bridge) resolveInProject(p string) (string, error) {
	root := b.projectDir()
	if root == "" {
		return "", errors.New("no hay proyecto activo")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	abs := strings.TrimSpace(p)
	if abs == "" {
		return filepath.Clean(rootAbs), nil
	}
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(rootAbs, abs)
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("ruta fuera del proyecto: %s", p)
	}
	return abs, nil
}

// ListDir lista un nivel de un directorio del proyecto. El arbol del
// explorador es perezoso: cada carpeta se pide al desplegarla, asi se puede
// navegar a cualquier profundidad sin cargar el proyecto entero.
func (b *Bridge) ListDir(dir string) []*FileNode {
	abs, err := b.resolveInProject(dir)
	if err != nil {
		return []*FileNode{}
	}
	root, err := b.resolveInProject("")
	if err != nil {
		return []*FileNode{}
	}
	return listDir(root, abs)
}

func listDir(root, dir string) []*FileNode {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []*FileNode{}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
	out := []*FileNode{}
	for _, ent := range entries {
		name := ent.Name()
		if ent.IsDir() && skipDirs[name] {
			continue
		}
		if strings.HasPrefix(name, ".") && name != ".gitignore" {
			continue
		}
		full := filepath.Join(dir, name)
		rel, err := filepath.Rel(root, full)
		if err != nil {
			rel = name
		}
		out = append(out, &FileNode{
			Name:  name,
			Path:  full,
			Rel:   filepath.ToSlash(rel),
			IsDir: ent.IsDir(),
		})
	}
	return out
}

// FileDelete borra un archivo o una carpeta (recursivo) del proyecto.
func (b *Bridge) FileDelete(path string) error {
	abs, err := b.resolveInProject(path)
	if err != nil {
		return err
	}
	if root, err := b.resolveInProject(""); err == nil && abs == root {
		return errors.New("no se puede borrar la raiz del proyecto")
	}
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("no existe: %s", filepath.Base(abs))
	}
	if err := os.RemoveAll(abs); err != nil {
		return fmt.Errorf("no se pudo borrar %s: %w", filepath.Base(abs), err)
	}
	return nil
}

// FileRename cambia el nombre de un elemento dentro de su misma carpeta.
// newName debe ser un nombre simple: sin separadores ni "..".
func (b *Bridge) FileRename(path, newName string) (string, error) {
	abs, err := b.resolveInProject(path)
	if err != nil {
		return "", err
	}
	newName = strings.TrimSpace(newName)
	if newName == "" || newName == "." || newName == ".." ||
		strings.ContainsAny(newName, `/\:*?"<>|`) {
		return "", errors.New("nombre no valido")
	}
	if root, err := b.resolveInProject(""); err == nil && abs == root {
		return "", errors.New("no se puede renombrar la raiz del proyecto")
	}
	dest := filepath.Join(filepath.Dir(abs), newName)
	if dest == abs {
		return abs, nil
	}
	if _, err := os.Stat(dest); err == nil {
		return "", fmt.Errorf("ya existe %q en esa carpeta", newName)
	}
	if err := os.Rename(abs, dest); err != nil {
		return "", fmt.Errorf("no se pudo renombrar: %w", err)
	}
	return dest, nil
}

// FileCopy copia archivos o carpetas a destDir (dentro del proyecto).
func (b *Bridge) FileCopy(paths []string, destDir string) error {
	return b.transfer(paths, destDir, false)
}

// FileMove mueve archivos o carpetas a destDir (dentro del proyecto).
func (b *Bridge) FileMove(paths []string, destDir string) error {
	return b.transfer(paths, destDir, true)
}

// transfer implementa copiar y cortar. Todo se valida contra la raiz del
// proyecto y se rechazan los destinos que caen dentro del propio origen (eso
// generaria una copia infinita).
func (b *Bridge) transfer(paths []string, destDir string, move bool) error {
	if len(paths) == 0 {
		return errors.New("no hay nada que pegar")
	}
	dest, err := b.resolveInProject(destDir)
	if err != nil {
		return err
	}
	if info, err := os.Stat(dest); err != nil || !info.IsDir() {
		return errors.New("el destino no es una carpeta")
	}
	for _, p := range paths {
		src, err := b.resolveInProject(p)
		if err != nil {
			return err
		}
		if src == dest {
			return errors.New("el destino es el mismo archivo")
		}
		if strings.HasPrefix(dest, src+string(filepath.Separator)) {
			return fmt.Errorf("no se puede pegar %s dentro de si mismo", filepath.Base(src))
		}
		target := filepath.Join(dest, filepath.Base(src))
		if target == src {
			continue
		}
		if _, err := os.Stat(target); err == nil {
			return fmt.Errorf("ya existe %q en el destino", filepath.Base(src))
		}
		if move {
			if err := os.Rename(src, target); err == nil {
				continue
			}
			// Distinto volumen o destino ocupado: se copia y se borra el origen.
			if err := copyPath(src, target); err != nil {
				return err
			}
			if err := os.RemoveAll(src); err != nil {
				return err
			}
			continue
		}
		if err := copyPath(src, target); err != nil {
			return err
		}
	}
	return nil
}

// copyPath copia un archivo o un arbol de carpetas.
func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyTree(src, dst)
	}
	return copyFileMode(src, dst, info.Mode())
}

func copyTree(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, ent := range entries {
		if err := copyPath(filepath.Join(src, ent.Name()), filepath.Join(dst, ent.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFileMode(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
