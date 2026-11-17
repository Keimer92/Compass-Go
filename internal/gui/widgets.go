package gui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// closeButton es la X circular estilo macOS para cerrar diálogos y paneles.
func closeButton(gtx layout.Context, th *material.Theme, p Palette,
	btn *widget.Clickable) layout.Dimensions {

	return closeButtonSize(gtx, th, p, btn, unit.Dp(32))
}

// tabCloseButton es la X de una pestana de terminal, mas pequena.
func tabCloseButton(gtx layout.Context, th *material.Theme, p Palette,
	btn *widget.Clickable) layout.Dimensions {

	return closeButtonSize(gtx, th, p, btn, unit.Dp(22))
}

func closeButtonSize(gtx layout.Context, th *material.Theme, p Palette,
	btn *widget.Clickable, dpSize unit.Dp) layout.Dimensions {

	size := gtx.Dp(dpSize)
	red := color.NRGBA{R: 0xff, G: 0x5f, B: 0x57, A: 0xff}
	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		rad := size / 2
		bg := red
		if !btn.Hovered() {
			bg = blend(p.Background, red, 0.82)
		}
		stack := clip.RRect{
			Rect: image.Rect(0, 0, size, size),
			SE: rad, SW: rad, NE: rad, NW: rad,
		}.Push(gtx.Ops)
		paint.Fill(gtx.Ops, bg)
		stack.Pop()

		ig := gtx
		ig.Constraints = layout.Exact(image.Pt(size, size))
		lbl := material.Label(th, 22, "×")
		lbl.Alignment = text.Middle
		lbl.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
		lbl.Layout(ig)
		return layout.Dimensions{Size: image.Pt(size, size)}
	})
}

// dialogTitle dibuja el titulo de un dialogo con la X a la derecha.
func dialogTitle(gtx layout.Context, a *Application, p Palette, title string,
	close *widget.Clickable) layout.Dimensions {

	return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body1(a.th, title)
				lbl.Color = p.Brand
				return lbl.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return closeButton(gtx, a.th, p, close)
			}),
		)
	})
}
