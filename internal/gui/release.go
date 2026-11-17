package gui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	mdicons "golang.org/x/exp/shiny/materialdesign/icons"
)

func cardAccent(p Palette, key string) color.NRGBA {
	for i, k := range themeCardKeys {
		if k == key && i < len(p.Cards) {
			return p.Cards[i]
		}
	}
	return p.Cards[0]
}

// ReleaseOption es una subcard del menu de release, ahora nativa y clicable.
type ReleaseOption struct {
	Task     string
	Label    string
	Desc     string
	ColorKey string
	Icon     *widget.Icon
}

// ReleaseDialog reemplaza el submenu de tcell de los releases.
type ReleaseDialog struct {
	Open    bool
	Title   string
	Options []ReleaseOption
	clicks  []widget.Clickable
	close   widget.Clickable
}

var releaseIcons = buildReleaseIcons()

func buildReleaseIcons() map[string]*widget.Icon {
	keys := map[string][]byte{
		"run":     mdicons.AVPlayArrow,
		"test":    mdicons.ToggleStar,
		"build":   mdicons.ActionBuild,
		"install": mdicons.ActionGetApp,
		"android": mdicons.ActionAndroid,
		"apk32":   mdicons.ActionAssignment,
		"apk":     mdicons.ActionAssignment,
		"aab":     mdicons.MapsLayers,
		"clean":   mdicons.ActionDelete,
		"git":     mdicons.ActionCode,
		"debug":   mdicons.ActionSearch,
		"environment": mdicons.ActionSettings,
		"version": mdicons.ActionUpdate,
	}
	out := make(map[string]*widget.Icon, len(keys))
	for k, data := range keys {
		if ic, err := widget.NewIcon(data); err == nil {
			out[k] = ic
		}
	}
	return out
}

// newReleaseDialog crea el dialogo segun el tipo de release.
func newReleaseDialog(title string, options []ReleaseOption) *ReleaseDialog {
	d := &ReleaseDialog{
		Open: true, Title: title, Options: options,
		clicks: make([]widget.Clickable, len(options)),
	}
	for i := range options {
		if options[i].Icon == nil {
			options[i].Icon = releaseIcons[options[i].ColorKey]
		}
	}
	return d
}

func windowsReleaseOptions() []ReleaseOption {
	return []ReleaseOption{
		{"windows-release-x64", "Windows x64", "64 bits · uso general", "run", nil},
		{"windows-release-all", "Todos", "Compila x86 + x64 + ARM64", "test", nil},
		{"windows-release-x86", "Windows x86", "32 bits · PCs y VMs antiguas", "build", nil},
		{"windows-release-arm64", "Windows ARM64", "Snapdragon X · bajo consumo", "install", nil},
	}
}

func androidReleaseOptions() []ReleaseOption {
	return []ReleaseOption{
		{"android-release-arm64", "Android arm64", "arm64-v8a · móviles modernos", "android", nil},
		{"android-release-armv7", "Android armv7", "armeabi-v7a · equipos antiguos", "apk32", nil},
		{"android-release-universal", "Android universal", "Una APK con todas las ABIs", "apk", nil},
		{"android-release-all", "Todos", "Compila las tres APK en serie", "aab", nil},
	}
}

func gitOptions() []ReleaseOption {
	return []ReleaseOption{
		{"git-do-init", "Inicializar repositorio", "git init + rama main + primer commit", "build", nil},
		{"git-do-gitignore", "Crear/actualizar .gitignore", "Plantilla por tipo de proyecto", "clean", nil},
		{"git-do-remote", "Conectar con remoto", "Enlazar con GitHub (URL o gh)", "git", nil},
		{"git-do-status", "Ver estado", "Rama, remoto y cambios", "debug", nil},
	}
}

func envOptions() []ReleaseOption {
	return []ReleaseOption{
		{"env-do-backup", "Backup completo", "Variables, inventario y archivos", "environment", nil},
		{"env-do-inventory", "Solo inventario", "Herramientas y versiones, sin copiar", "version", nil},
		{"env-do-restore", "Restaurar entorno", "Variables, rutas y rollback", "clean", nil},
		{"env-do-validate", "Validar backup", "Diff sin modificar nada", "debug", nil},
		{"env-do-open", "Abrir respaldos", "Carpeta de respaldos de Compass", "git", nil},
	}
}

// Layout dibuja el dialogo de release como overlay.
func (d *ReleaseDialog) Layout(gtx layout.Context, a *Application, p Palette, w, h int) {
	pw, ph := gtx.Dp(unit.Dp(660)), gtx.Dp(unit.Dp(430))
	px, py := (w-pw)/2, (h-ph)/2
	panel := image.Rect(px, py, px+pw, py+ph)

	dim := clip.Rect(image.Rect(0, 0, w, h)).Push(gtx.Ops)
	paint.Fill(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 0xaa})
	dim.Pop()
	rad := gtx.Dp(unit.Dp(14))
	stack := clip.RRect{Rect: panel, SE: rad, SW: rad, NE: rad, NW: rad}.Push(gtx.Ops)
	paint.Fill(gtx.Ops, p.Background)
	stack.Pop()

	bg := gtx
	bg.Constraints = layout.Exact(panel.Size())
	tr := op.Offset(panel.Min).Push(bg.Ops)
	layout.Inset{Top: unit.Dp(18), Left: unit.Dp(22), Right: unit.Dp(22)}.Layout(bg,
		func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return dialogTitle(gtx, a, p, d.Title, &d.close)
				}),
			}
			for i := range d.Options {
				i := i
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return d.optionCard(gtx, a, p, i)
				}))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	tr.Pop()
}

func (d *ReleaseDialog) optionCard(gtx layout.Context, a *Application,
	p Palette, i int) layout.Dimensions {

	opt := d.Options[i]
	accent := cardAccent(p, opt.ColorKey)
	hovered := d.clicks[i].Hovered()
	bg := blend(p.Background, p.Border, 0.28)
	if hovered {
		bg = dim(accent, 38)
	}
	return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return d.clicks[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			rad := gtx.Dp(unit.Dp(9))
			outer := clip.RRect{
				Rect: image.Rectangle{Max: gtx.Constraints.Max},
				SE: rad, SW: rad, NE: rad, NW: rad,
			}.Push(gtx.Ops)
			paint.Fill(gtx.Ops, bg)
			outer.Pop()

			return layout.Inset{Left: unit.Dp(14), Top: unit.Dp(10), Bottom: unit.Dp(10)}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if opt.Icon == nil {
								return layout.Dimensions{}
							}
							is := gtx.Dp(unit.Dp(26))
							ig := gtx
							ig.Constraints = layout.Exact(image.Pt(is, is))
							opt.Icon.Layout(ig, accent)
							return layout.Dimensions{Size: image.Pt(is, is)}
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Left: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										lbl := material.Body1(a.th, opt.Label)
										lbl.Color = p.Text
										return lbl.Layout(gtx)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										lbl := material.Caption(a.th, opt.Desc)
										lbl.Color = p.Muted
										return lbl.Layout(gtx)
									}),
								)
							})
						}),
					)
				})
		})
	})
}
