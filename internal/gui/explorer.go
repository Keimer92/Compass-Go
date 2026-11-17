package gui

import (
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"compass/internal/windialog"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// GitChange es una linea de `git status --porcelain`.
type GitChange struct {
	Code string
	Path string
}

// FileNode es un nodo del arbol de archivos.
type FileNode struct {
	Name     string
	Path     string
	IsDir    bool
	Depth    int
	Children []*FileNode
}

// Explorer es el panel derecho: cambios de Git y arbol de archivos.
type Explorer struct {
	root     string
	mu       sync.RWMutex
	tree     []*FileNode
	changes  []GitChange
	expanded map[string]bool

	refreshBtn widget.Clickable
	rows       map[string]*widget.Clickable
}

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "bin": true, "obj": true,
	"dist": true, "vendor": true, "target": true, ".idea": true,
	".vs": true, "corekit": true, "out": true,
}

// NewExplorer crea el panel sobre la raiz del proyecto.
func NewExplorer(root string) *Explorer {
	e := &Explorer{
		root:     root,
		expanded: map[string]bool{},
		rows:     make(map[string]*widget.Clickable),
	}
	// Raiz y primer nivel abiertos por defecto.
	e.expanded[root] = true
	e.Refresh()
	return e
}

// Refresh reconstruye el arbol y vuelve a consultar git. Se llama desde una
// goroutine del pulso de la GUI.
func (e *Explorer) Refresh() {
	tree := e.buildTree()
	changes := gitChanges(e.root)
	e.mu.Lock()
	e.tree = tree
	e.changes = changes
	e.mu.Unlock()
}

func (e *Explorer) buildTree() []*FileNode {
	entries, err := os.ReadDir(e.root)
	if err != nil {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.IsDir() != b.IsDir() {
			return a.IsDir()
		}
		return strings.ToLower(a.Name()) < strings.ToLower(b.Name())
	})
	var out []*FileNode
	for _, ent := range entries {
		name := ent.Name()
		if ent.IsDir() && skipDirs[name] {
			continue
		}
		if strings.HasPrefix(name, ".") && name != ".gitignore" {
			continue
		}
		node := &FileNode{
			Name:  name,
			Path:  filepath.Join(e.root, name),
			IsDir: ent.IsDir(),
			Depth: 0,
		}
		if ent.IsDir() && e.expanded[node.Path] {
			node.Children = e.readChildren(node.Path, 1)
		}
		out = append(out, node)
		if len(out) >= 120 {
			break
		}
	}
	return out
}

func (e *Explorer) readChildren(dir string, depth int) []*FileNode {
	if depth > 3 {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.IsDir() != b.IsDir() {
			return a.IsDir()
		}
		return strings.ToLower(a.Name()) < strings.ToLower(b.Name())
	})
	var out []*FileNode
	for _, ent := range entries {
		name := ent.Name()
		if ent.IsDir() && skipDirs[name] {
			continue
		}
		if strings.HasPrefix(name, ".") {
			continue
		}
		node := &FileNode{
			Name:  name,
			Path:  filepath.Join(dir, name),
			IsDir: ent.IsDir(),
			Depth: depth,
		}
		if ent.IsDir() && e.expanded[node.Path] {
			node.Children = e.readChildren(node.Path, depth+1)
		}
		out = append(out, node)
		if len(out) >= 60 {
			break
		}
	}
	return out
}

func gitChanges(root string) []GitChange {
	cmd := exec.Command("git", "status", "--porcelain=v1", "-uall")
	windialog.NoWindow(cmd)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var changes []GitChange
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 4 {
			continue
		}
		code := strings.TrimSpace(line[:2])
		if code == "" {
			continue
		}
		changes = append(changes, GitChange{Code: code, Path: strings.TrimSpace(line[3:])})
		if len(changes) >= 100 {
			break
		}
	}
	return changes
}

// gitStatusInfo traduce el codigo porcelain de git a una letra, un estado
// legible y un color, como hace la vista de Source Control de VS Code.
func gitStatusInfo(code string, p Palette) (letter, status string, col color.NRGBA) {
	// git porcelain: columna 0 = indice (staged), columna 1 = working tree.
	idx, work := byte(' '), byte(' ')
	if len(code) > 0 {
		idx = code[0]
	}
	if len(code) > 1 {
		work = code[1]
	}
	switch {
	case idx == '?' || work == '?':
		return "U", "Untracked", p.Success
	case work == 'D' || idx == 'D':
		return "D", "Deleted", color.NRGBA{R: 0xff, G: 0x6b, B: 0x6b, A: 0xff}
	case work == 'M':
		return "M", "Modified", p.Warning
	case idx == 'M':
		return "M", "Staged", p.Warning
	case idx == 'A':
		return "A", "Added", p.Success
	default:
		return "?", "Changed", p.Muted
	}
}

// Layout dibuja el panel dentro de gtx.Constraints.Max.
func (e *Explorer) Layout(gtx layout.Context, p Palette, th *material.Theme) layout.Dimensions {
	dims := layout.Dimensions{Size: gtx.Constraints.Max}
	stack := clip.Rect(image.Rectangle{Max: dims.Size}).Push(gtx.Ops)
	defer stack.Pop()

	e.mu.RLock()
	tree, changes := e.tree, e.changes
	e.mu.RUnlock()

	if e.refreshBtn.Clicked(gtx) {
		go e.Refresh()
	}

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return headerRow(gtx, th, p, "EXPLORADOR", &e.refreshBtn)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Caption(th, "CAMBIOS GIT ("+itoaSimple(len(changes))+")")
			lbl.Color = p.Muted
			return sectionRow(gtx, lbl.Layout)
		}),
	}
	shown := changes
	if len(shown) > 14 {
		shown = shown[:14]
	}
	for _, ch := range shown {
		ch := ch
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return changeRow(gtx, th, p, ch)
		}))
	}
	if len(changes) > len(shown) {
		more := changes[len(shown):]
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Caption(th, "  + "+itoaSimple(len(more))+" cambios")
			lbl.Color = p.Muted
			return lbl.Layout(gtx)
		}))
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		lbl := material.Caption(th, "ARCHIVOS")
		lbl.Color = p.Muted
		return sectionRow(gtx, lbl.Layout)
	}))
	for _, node := range flattenTree(tree, 24) {
		node := node
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return e.fileRow(gtx, th, p, node)
		}))
	}

	layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	return dims
}

func flattenTree(nodes []*FileNode, max int) []*FileNode {
	var out []*FileNode
	var walk func([]*FileNode)
	walk = func(list []*FileNode) {
		for _, n := range list {
			if len(out) >= max {
				return
			}
			out = append(out, n)
			walk(n.Children)
		}
	}
	walk(nodes)
	return out
}

func (e *Explorer) fileRow(gtx layout.Context, th *material.Theme,
	p Palette, node *FileNode) layout.Dimensions {

	btn := e.rows[node.Path]
	if btn == nil {
		btn = &widget.Clickable{}
		e.rows[node.Path] = btn
	}
	if btn.Clicked(gtx) && node.IsDir {
		e.expanded[node.Path] = !e.expanded[node.Path]
		go e.Refresh()
	}
	arrow := " "
	if node.IsDir {
		if e.expanded[node.Path] {
			arrow = "▾"
		} else {
			arrow = "▸"
		}
	}
	lbl := material.Caption(th, strings.Repeat("  ", node.Depth)+arrow+" "+node.Name)
	if node.IsDir {
		lbl.Color = p.Text
	} else {
		lbl.Color = p.Muted
	}
	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(10), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, lbl.Layout)
	})
}

func changeRow(gtx layout.Context, th *material.Theme, p Palette,
	ch GitChange) layout.Dimensions {

	letter, status, col := gitStatusInfo(ch.Code, p)
	return layout.Inset{Left: unit.Dp(10), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					sz := gtx.Dp(unit.Dp(17))
					s := clip.RRect{
						Rect: image.Rect(0, 0, sz, sz),
						SE: sz / 2, SW: sz / 2, NE: sz / 2, NW: sz / 2,
					}.Push(gtx.Ops)
					paint.Fill(gtx.Ops, blend(p.Background, col, 0.55))
					s.Pop()
					lbl := material.Caption(th, letter)
					lbl.Color = col
					lbl.Alignment = text.Middle
					ig := gtx
					ig.Constraints = layout.Exact(image.Pt(sz, sz))
					lbl.Layout(ig)
					return layout.Dimensions{Size: image.Pt(sz, sz)}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Caption(th, filepath.Base(ch.Path))
						lbl.Color = p.Text
						return lbl.Layout(gtx)
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Caption(th, status)
					lbl.Color = col
					return lbl.Layout(gtx)
				}),
			)
		})
}

func headerRow(gtx layout.Context, th *material.Theme, p Palette,
	title string, refreshBtn *widget.Clickable) layout.Dimensions {

	return layout.Inset{Left: unit.Dp(12), Top: unit.Dp(12), Bottom: unit.Dp(8), Right: unit.Dp(10)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Body2(th, title)
					lbl.Color = p.Brand
					return lbl.Layout(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Caption(th, "↻")
					lbl.Color = p.Muted
					return refreshBtn.Layout(gtx, lbl.Layout)
				}),
			)
		})
}

func sectionRow(gtx layout.Context, w layout.Widget) layout.Dimensions {
	return layout.Inset{Left: unit.Dp(12), Top: unit.Dp(12), Bottom: unit.Dp(4)}.Layout(gtx, w)
}

func itoaSimple(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
