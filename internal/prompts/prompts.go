// Package prompts implementa los selectores interactivos (equivalente de
// src/utils/context.rs::pick_project_root, que en Rust usaba la crate inquire).
//
// Por ahora usa E/S de linea simple por stdin/stdout; en una fase posterior se
// puede sustituir por una UI con huh/bubbletea sin cambiar la firma.
package prompts

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"compass/internal/project"
)

// Select muestra opciones numeradas y devuelve el indice elegido (0-based).
func Select(title string, options []string) (int, error) {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Printf("║  %s\n", title)
	fmt.Println("╚══════════════════════════════════════════════════╝")
	for i, o := range options {
		fmt.Printf("  %2d. %s\n", i+1, o)
	}
	fmt.Print("\nOpcion: ")
	line, err := readLine()
	if err != nil {
		return -1, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(options) {
		return -1, fmt.Errorf("opción inválida")
	}
	return n - 1, nil
}

// Confirm pregunta si/no; devuelve def si la linea esta vacia.
func Confirm(prompt string, def bool) (bool, error) {
	suffix := " [s/N]: "
	if def {
		suffix = " [S/n]: "
	}
	fmt.Printf("%s%s", prompt, suffix)
	line, err := readLine()
	if err != nil {
		return def, err
	}
	line = strings.ToLower(strings.TrimSpace(line))
	if line == "" {
		return def, nil
	}
	return line == "s" || line == "si" || line == "sí" || line == "y" || line == "yes", nil
}

type action int

const (
	actUse action = iota
	actUp
	actDescend
	actEnter
)

// PickProjectRoot abre un navegador interactivo para elegir la carpeta de un
// proyecto, subiendo o entrando en subdirectorios. Devuelve "" si se cancela.
func PickProjectRoot(start string) (string, error) {
	current := start
	for {
		hereType, here := project.ClassifyDir(current)

		var options []string
		var actions []action
		var subPaths []string // rutas de las carpetas, en el mismo orden que options

		if here {
			options = append(options, fmt.Sprintf("✔  USAR ESTA CARPETA  (%s)", hereType.Label()))
			actions = append(actions, actUse)
		}
		parent := filepath.Dir(current)
		if parent != current {
			options = append(options, "↰  Subir al directorio padre")
			actions = append(actions, actUp)
		}

		entries, _ := os.ReadDir(current)
		type sub struct {
			path string
			pt   project.ProjectType
			isP  bool
		}
		var vis, hid []sub
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join(current, e.Name())
			pt, ok := project.ClassifyDir(p)
			s := sub{path: p, pt: pt, isP: ok}
			if strings.HasPrefix(e.Name(), ".") {
				hid = append(hid, s)
			} else {
				vis = append(vis, s)
			}
		}
		sort.Slice(vis, func(i, j int) bool {
			return strings.ToLower(filepath.Base(vis[i].path)) < strings.ToLower(filepath.Base(vis[j].path))
		})
		sort.Slice(hid, func(i, j int) bool {
			return strings.ToLower(filepath.Base(hid[i].path)) < strings.ToLower(filepath.Base(hid[j].path))
		})
		for _, s := range append(vis, hid...) {
			name := filepath.Base(s.path)
			if s.isP {
				options = append(options, fmt.Sprintf("  📂 %s    (%s)", name, s.pt.Label()))
				actions = append(actions, actEnter)
			} else {
				options = append(options, fmt.Sprintf("  📂 %s", name))
				actions = append(actions, actDescend)
			}
			subPaths = append(subPaths, s.path)
		}

		if len(options) == 0 {
			fmt.Println()
			fmt.Printf("[INFO] No hay subdirectorios en: %s\n", current)
			ok, err := Confirm("¿Salir sin elegir proyecto?", true)
			if err != nil {
				return "", err
			}
			if ok {
				return "", nil
			}
			continue
		}

		shown := current
		if len(shown) > 96 {
			shown = "…" + shown[len(shown)-95:]
		}
		fmt.Printf("\n  📂 %s\n", shown)

		idx, err := Select("SELECCIONA EL PROYECTO", options)
		if err != nil {
			return "", nil // invalido = cancelar
		}
		switch actions[idx] {
		case actUse:
			return current, nil
		case actUp:
			current = parent
		case actDescend, actEnter:
			// Las opciones fijas (USAR/SUBIR) vienen antes de las carpetas.
			fixed := 0
			if here {
				fixed++
			}
			if parent != current {
				fixed++
			}
			subIdx := idx - fixed
			if subIdx >= 0 && subIdx < len(subPaths) {
				current = subPaths[subIdx]
			}
		}
	}
}

func readLine() (string, error) {
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
