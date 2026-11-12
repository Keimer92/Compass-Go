// Package theme contiene los temas del TUI (inspirados en los de OpenCode) y
// su persistencia. Es el equivalente de src/theme.rs.
//
// Cada Theme es un conjunto de roles semanticos (no colores sueltos) para que el
// dibujado del menu no tenga literales de color: cambiar de tema solo cambia
// esta tabla.
package theme

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
)

// CardKeys son las claves de los acentos por card, en el orden del arreglo
// Theme.CardHexes. Una clave por MenuItem. El orden es load-bearing: coincide
// posicion a posicion con los 18 hex de cada tema.
var CardKeys = [18]string{
	// DEV
	"run", "android", "reload", "build", "apk", "apk32", "aab", "install", "profile", "test",
	// MNT
	"clean", "debug", "sign", "version",
	// CFG
	"git", "whatsapp", "terminal", "environment",
}

// Theme es el conjunto de colores del TUI.
type Theme struct {
	Name string

	Text       tcell.Color
	Muted      tcell.Color
	Border     tcell.Color
	Brand      tcell.Color
	Dev        tcell.Color
	Mnt        tcell.Color
	Cfg        tcell.Color
	HoverBg    tcell.Color
	Background tcell.Color
	Success    tcell.Color
	Info       tcell.Color
	Accent     tcell.Color
	Warning    tcell.Color

	// CardHexes es el acento propio de cada card (0xRRGGBB), en el orden de
	// CardKeys.
	CardHexes [18]int32
}

// hex convierte un literal 0xRRGGBB en color tcell.
func hex(v int32) tcell.Color { return tcell.NewHexColor(v) }

// dark oscurece v hacia el negro al pct por ciento. Es el tinte que se usa como
// fondo de la card seleccionada.
func dark(v int32, pct int32) tcell.Color {
	r := (v >> 16) & 0xff
	g := (v >> 8) & 0xff
	b := v & 0xff
	return tcell.NewRGBColor(r*pct/100, g*pct/100, b*pct/100)
}

// Section devuelve el color de acento de una seccion ("DEV" | "MNT" | "CFG").
// Cualquier otro valor cae en Cfg.
func (t *Theme) Section(section string) tcell.Color {
	switch section {
	case "DEV":
		return t.Dev
	case "MNT":
		return t.Mnt
	default:
		return t.Cfg
	}
}

func (t *Theme) cardHex(key string) int32 {
	idx := -1
	for i, k := range CardKeys {
		if k == key {
			idx = i
			break
		}
	}
	if idx < 0 {
		// Clave desconocida: cae en "terminal".
		for i, k := range CardKeys {
			if k == "terminal" {
				idx = i
				break
			}
		}
		if idx < 0 {
			idx = len(CardKeys) - 1
		}
	}
	return t.CardHexes[idx]
}

// CardAccent devuelve el color distintivo de una card (key sale de CardKeys).
func (t *Theme) CardAccent(key string) tcell.Color { return hex(t.cardHex(key)) }

// CardBg es el fondo de esa card cuando esta seleccionada: su acento apagado al
// 27 %.
func (t *Theme) CardBg(key string) tcell.Color { return dark(t.cardHex(key), 27) }

// Mix interpola dos colores (t=0 -> a, t=1 -> b).
func Mix(a, b tcell.Color, t float32) tcell.Color {
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	ar, ag, ab := a.RGB()
	br, bg, bb := b.RGB()
	lerp := func(x, y int32) int32 {
		return int32(float32(x) + (float32(y)-float32(x))*t + 0.5)
	}
	return tcell.NewRGBColor(lerp(ar, br), lerp(ag, bg), lerp(ab, bb))
}

// Themes es el temario. El orden es el de Ctrl+T; Compass va primero.
var Themes = []*Theme{
	{
		Name:       "Compass",
		Text:       tcell.ColorWhite,
		Muted:      tcell.ColorGray,
		Border:     tcell.ColorGray,
		Brand:      tcell.ColorGreen,
		Dev:        tcell.ColorTeal,
		Mnt:        tcell.ColorOlive,
		Cfg:        tcell.ColorPurple,
		HoverBg:    hex(0x1e1e24),
		Background: hex(0x16161e),
		Success:    tcell.ColorGreen,
		Info:       tcell.ColorTeal,
		Accent:     tcell.ColorPurple,
		Warning:    tcell.ColorOlive,
		CardHexes: [18]int32{
			0x00abcd, 0x00ffff, 0x5fd7ff, 0xafff00, 0xffd700, 0xffa500, 0xff87d7, 0x00d7af,
			0xffe79f, 0xffff00, 0xffcc2f, 0xff6f6f, 0xffb72f, 0xff4f4f, 0xff00ff, 0xd787ff,
			0xaf87ff, 0x00d7ff,
		},
	},
	{
		Name:       "OpenCode",
		Text:       hex(0xeeeeee),
		Muted:      hex(0x808080),
		Border:     hex(0x484848),
		Brand:      hex(0xfab283),
		Dev:        hex(0x56b6c2),
		Mnt:        hex(0xf5a742),
		Cfg:        hex(0x9d7cd8),
		HoverBg:    hex(0x1e1e1e),
		Background: hex(0x131313),
		Success:    hex(0x7fd88f),
		Info:       hex(0x56b6c2),
		Accent:     hex(0x9d7cd8),
		Warning:    hex(0xf5a742),
		CardHexes: [18]int32{
			0x56b6c2, 0x61afef, 0x98c379, 0xe5c07b, 0xe06c75, 0xff5c93, 0xd19a66, 0x7fd88f,
			0xef6c75, 0xc678dd, 0xf5a742, 0x9d7cd8, 0xfab283, 0xffb86c, 0xbd93f9, 0x2bbac5,
			0xa8a1ff, 0x89ddff,
		},
	},
	{
		Name:       "Tokyo Night",
		Text:       hex(0xc8d3f5),
		Muted:      hex(0x828bb8),
		Border:     hex(0x545c7e),
		Brand:      hex(0x82aaff),
		Dev:        hex(0x82aaff),
		Mnt:        hex(0xffc777),
		Cfg:        hex(0xc099ff),
		HoverBg:    hex(0x222436),
		Background: hex(0x1a1b26),
		Success:    hex(0xc3e88d),
		Info:       hex(0x86e1fc),
		Accent:     hex(0xc099ff),
		Warning:    hex(0xffc777),
		CardHexes: [18]int32{
			0x7aa2f7, 0x86e1fc, 0xc3e88d, 0xffc777, 0xf7768e, 0xdb5c3c, 0x2ac3de, 0x73daca,
			0xbb9af7, 0xe0af68, 0xff9e64, 0x7dcfff, 0xffa29b, 0x7cf6b5, 0xb8b2fa, 0xad7ea8,
			0x9d7cd8, 0x2ac3de,
		},
	},
	{
		Name:       "Catppuccin",
		Text:       hex(0xcdd6f4),
		Muted:      hex(0x7f849c),
		Border:     hex(0x313244),
		Brand:      hex(0x89b4fa),
		Dev:        hex(0x89b4fa),
		Mnt:        hex(0xf9e2af),
		Cfg:        hex(0xcba6f7),
		HoverBg:    hex(0x313244),
		Background: hex(0x1e1e2e),
		Success:    hex(0xa6e3a1),
		Info:       hex(0x94e2d5),
		Accent:     hex(0xcba6f7),
		Warning:    hex(0xf9e2af),
		CardHexes: [18]int32{
			0x89b4fa, 0x94e2d5, 0xa6e3a1, 0xf9e2af, 0xf38ba8, 0xe78284, 0xf2cdcd, 0x94b8ff,
			0xf5c2e7, 0xfab387, 0xeba069, 0xb4befe, 0xcba6f7, 0xffa657, 0xa0c4ff, 0x7dcfff,
			0x89d88b, 0xffa657,
		},
	},
	{
		Name:       "Dracula",
		Text:       hex(0xf8f8f2),
		Muted:      hex(0x6272a4),
		Border:     hex(0x44475a),
		Brand:      hex(0xbd93f9),
		Dev:        hex(0x8be9fd),
		Mnt:        hex(0xf1fa8c),
		Cfg:        hex(0xff79c6),
		HoverBg:    hex(0x3a3c4e),
		Background: hex(0x282a36),
		Success:    hex(0x50fa7b),
		Info:       hex(0xffb86c),
		Accent:     hex(0xff79c6),
		Warning:    hex(0xf1fa8c),
		CardHexes: [18]int32{
			0x8be9fd, 0xbd93f9, 0x50fa7b, 0xf1fa8c, 0xff5555, 0xffb86c, 0x62d4a8, 0x3ee6c4,
			0xe3f9fb, 0x8aff80, 0xfabf9f, 0xff79c6, 0x69d0e7, 0x5aebad, 0xefe48d, 0xff92d0,
			0xf264c9, 0x8be9fd,
		},
	},
	{
		Name:       "Gruvbox",
		Text:       hex(0xebdbb2),
		Muted:      hex(0x928374),
		Border:     hex(0x504945),
		Brand:      hex(0xb8bb26),
		Dev:        hex(0x83a598),
		Mnt:        hex(0xfabd2f),
		Cfg:        hex(0xd3869b),
		HoverBg:    hex(0x3c3836),
		Background: hex(0x282828),
		Success:    hex(0xb8bb26),
		Info:       hex(0x83a598),
		Accent:     hex(0xd3869b),
		Warning:    hex(0xfe8019),
		CardHexes: [18]int32{
			0x83a598, 0x458588, 0xb8bb26, 0xfabd2f, 0xfb4934, 0xfe8019, 0xba5446, 0x8ec07c,
			0xdbe2d5, 0xa9b665, 0xf9f24a, 0xeceaea, 0xd3869b, 0xb16286, 0xd79921, 0xfbf1c7,
			0x689d6a, 0x458588,
		},
	},
	{
		Name:       "Nord",
		Text:       hex(0xeceff4),
		Muted:      hex(0x8b95a7),
		Border:     hex(0x434c5e),
		Brand:      hex(0x88c0d0),
		Dev:        hex(0x88c0d0),
		Mnt:        hex(0xebcb8b),
		Cfg:        hex(0xb48ead),
		HoverBg:    hex(0x3b4252),
		Background: hex(0x2e3440),
		Success:    hex(0xa3be8c),
		Info:       hex(0x81a1c1),
		Accent:     hex(0xb48ead),
		Warning:    hex(0xd08770),
		CardHexes: [18]int32{
			0x88c0d0, 0x81a1c1, 0xa3be8c, 0xebcb8b, 0xbf616a, 0xd08770, 0xe5e9f0, 0x8fbcbb,
			0x8fa8d8, 0xb48ead, 0x5e81a7, 0x87afaf, 0xc07866, 0x57909c, 0xd899c4, 0xaeb7df,
			0xc6d0dd, 0x88c0d0,
		},
	},
	{
		Name:       "One Dark",
		Text:       hex(0xabb2bf),
		Muted:      hex(0x5c6370),
		Border:     hex(0x393f4a),
		Brand:      hex(0x61afef),
		Dev:        hex(0x61afef),
		Mnt:        hex(0xe5c07b),
		Cfg:        hex(0xc678dd),
		HoverBg:    hex(0x353b45),
		Background: hex(0x282c34),
		Success:    hex(0x98c379),
		Info:       hex(0x56b6c2),
		Accent:     hex(0xc678dd),
		Warning:    hex(0xe5c07b),
		CardHexes: [18]int32{
			0x61afef, 0x56b6c2, 0x3ece9d, 0xe5c07b, 0xe06c75, 0xd19a66, 0xbe5046, 0x98c379,
			0xc678dd, 0xb8d17a, 0x7ae0c0, 0xef7d57, 0x9d7cd8, 0xef6ac0, 0x8fa3bf, 0x25d366,
			0x7f848e, 0x61afef,
		},
	},
	{
		Name:       "Monokai",
		Text:       hex(0xf8f8f2),
		Muted:      hex(0x75715e),
		Border:     hex(0x3e3d32),
		Brand:      hex(0xa6e22e),
		Dev:        hex(0x66d9ef),
		Mnt:        hex(0xe6db74),
		Cfg:        hex(0xae81ff),
		HoverBg:    hex(0x3e3d32),
		Background: hex(0x272822),
		Success:    hex(0xa6e22e),
		Info:       hex(0xfd971f),
		Accent:     hex(0xae81ff),
		Warning:    hex(0xe6db74),
		CardHexes: [18]int32{
			0x66d9ef, 0xae81ff, 0xa6e22e, 0xe6db74, 0xf92672, 0xfd971f, 0xff4040, 0x2ee6a8,
			0x8fd1e4, 0xf5f5a0, 0x70e0d0, 0xff9ec9, 0xc06fe0, 0x5ef38c, 0xffcb3d, 0x8be04e,
			0xb8b8b8, 0x66d9ef,
		},
	},
	{
		Name:       "Rosé Pine",
		Text:       hex(0xe0def4),
		Muted:      hex(0x6e6a86),
		Border:     hex(0x403d52),
		Brand:      hex(0xebbcba),
		Dev:        hex(0x9ccfd8),
		Mnt:        hex(0xf6c177),
		Cfg:        hex(0xc4a7e7),
		HoverBg:    hex(0x26233a),
		Background: hex(0x191724),
		Success:    hex(0x9ccfd8),
		Info:       hex(0x9ccfd8),
		Accent:     hex(0xc4a7e7),
		Warning:    hex(0xf6c177),
		CardHexes: [18]int32{
			0x9ccfd8, 0xc4a7e7, 0x31748f, 0xf6c177, 0xeb6c88, 0xe6976b, 0xf8bdc0, 0x8ec9a4,
			0xd8b4f0, 0xc2c2a8, 0x6bb8c4, 0xea84a2, 0xcba0f5, 0x5ec4a8, 0x9083a4, 0x6fcf9f,
			0xebbcba, 0x9ccfd8,
		},
	},
	{
		Name:       "Kanagawa",
		Text:       hex(0xdcd7ba),
		Muted:      hex(0x727169),
		Border:     hex(0x54546d),
		Brand:      hex(0x7e9cd8),
		Dev:        hex(0x7e9cd8),
		Mnt:        hex(0xd7a657),
		Cfg:        hex(0x957fb8),
		HoverBg:    hex(0x2a2a37),
		Background: hex(0x1f1f28),
		Success:    hex(0x98bb6c),
		Info:       hex(0x76946a),
		Accent:     hex(0xd27e99),
		Warning:    hex(0xd7a657),
		CardHexes: [18]int32{
			0x7e9cd8, 0x7fb4ca, 0x98bb6c, 0xd7a657, 0xe8246a, 0xcc8844, 0xd2787c, 0x80a66f,
			0xa3c48f, 0xc9b87a, 0x6da8c0, 0x76946a, 0x957fb8, 0xd4b08c, 0x5c8496, 0x8fd0a8,
			0xc0b8a0, 0x7e9cd8,
		},
	},
	{
		Name:       "Matrix",
		Text:       hex(0x62ff94),
		Muted:      hex(0x8ca391),
		Border:     hex(0x1e2a1b),
		Brand:      hex(0x2eff6a),
		Dev:        hex(0x62ff94),
		Mnt:        hex(0xe6ff57),
		Cfg:        hex(0xc770ff),
		HoverBg:    hex(0x141c12),
		Background: hex(0x0a120a),
		Success:    hex(0x2eff6a),
		Info:       hex(0x30b3ff),
		Accent:     hex(0xc770ff),
		Warning:    hex(0xe6ff57),
		CardHexes: [18]int32{
			0x62ff94, 0x30b3ff, 0x2eff6a, 0xe6ff57, 0xff4f7e, 0xff8a3d, 0x00e5b0, 0x7cffc4,
			0xb8ff6a, 0xffd800, 0x00b3a6, 0x57d8ff, 0xc770ff, 0x9dff7a, 0x00ffc8, 0xff6a6a,
			0xc0c0c0, 0x30b3ff,
		},
	},
	{
		Name:       "Synthwave 84",
		Text:       hex(0xffffff),
		Muted:      hex(0x848bbd),
		Border:     hex(0x495495),
		Brand:      hex(0xff7edb),
		Dev:        hex(0x36f9f6),
		Mnt:        hex(0xfede5d),
		Cfg:        hex(0xb084eb),
		HoverBg:    hex(0x2a2139),
		Background: hex(0x241b2f),
		Success:    hex(0x72f1b8),
		Info:       hex(0xff8b39),
		Accent:     hex(0xb084eb),
		Warning:    hex(0xfede5d),
		CardHexes: [18]int32{
			0x36f9f6, 0xff7edb, 0x72f1b8, 0xfede5d, 0xfe44f6, 0xff8b39, 0xfc297b, 0x00d9ff,
			0xb084eb, 0xd8f97f, 0x5580ff, 0xff9de7, 0x9bf6ff, 0xffc0cb, 0x05d9e0, 0x39ff14,
			0xcd6fff, 0x36f9f6,
		},
	},
	{
		Name:       "Cyberpunk",
		Text:       hex(0xe6e6ff),
		Muted:      hex(0x8b8bb8),
		Border:     hex(0x3b2a5a),
		Brand:      hex(0xff2a6d),
		Dev:        hex(0x00f0ff),
		Mnt:        hex(0xffd319),
		Cfg:        hex(0xb967ff),
		HoverBg:    hex(0x2a1240),
		Background: hex(0x12041f),
		Success:    hex(0x00ff9f),
		Info:       hex(0x00f0ff),
		Accent:     hex(0xff2a6d),
		Warning:    hex(0xffd319),
		CardHexes: [18]int32{
			0x00f0ff, 0xff2a6d, 0x00ff9f, 0xffd319, 0xff6b35, 0xff3864, 0x05d9e8, 0xd1f7ff,
			0xb967ff, 0xf9f871, 0x7a5cff, 0xff9de7, 0x00e5b0, 0xffd319, 0x2de2e6, 0xff2a6d,
			0x9d4edd, 0x00f0ff,
		},
	},
	{
		Name:       "Everforest",
		Text:       hex(0xd3c6aa),
		Muted:      hex(0x859289),
		Border:     hex(0x475258),
		Brand:      hex(0xa7c080),
		Dev:        hex(0x7fbbb3),
		Mnt:        hex(0xdbbc7f),
		Cfg:        hex(0xd699b6),
		HoverBg:    hex(0x3d484d),
		Background: hex(0x2d353b),
		Success:    hex(0xa7c080),
		Info:       hex(0x7fbbb3),
		Accent:     hex(0xd699b6),
		Warning:    hex(0xdbbc7f),
		CardHexes: [18]int32{
			0x7fbbb3, 0xa7c080, 0xdbbc7f, 0xe67e80, 0xe69875, 0xd699b6, 0x83c092, 0x9da9a0,
			0xd3c6aa, 0xa7c080, 0x7fbbb3, 0xdbbc7f, 0xe67e80, 0xe69875, 0xd699b6, 0x83c092,
			0x9da9a0, 0x7fbbb3,
		},
	},
	{
		Name:       "Night Owl",
		Text:       hex(0xd6deeb),
		Muted:      hex(0x637777),
		Border:     hex(0x1d3b53),
		Brand:      hex(0x82aaff),
		Dev:        hex(0x82aaff),
		Mnt:        hex(0xecc48d),
		Cfg:        hex(0xc792ea),
		HoverBg:    hex(0x1d3b53),
		Background: hex(0x011627),
		Success:    hex(0xaddb67),
		Info:       hex(0x7fdbca),
		Accent:     hex(0xc792ea),
		Warning:    hex(0xecc48d),
		CardHexes: [18]int32{
			0x82aaff, 0x7fdbca, 0xaddb67, 0xecc48d, 0xef5350, 0xf78c6c, 0xc792ea, 0x21c7a8,
			0xd6deeb, 0xffcb8e, 0x5f7e97, 0x82aaff, 0xc792ea, 0x7fdbca, 0xecc48d, 0xaddb67,
			0x637777, 0x82aaff,
		},
	},
	{
		Name:       "Cobalt2",
		Text:       hex(0xffffff),
		Muted:      hex(0x8f9dbb),
		Border:     hex(0x1f4662),
		Brand:      hex(0xffc600),
		Dev:        hex(0x2affdf),
		Mnt:        hex(0xffc600),
		Cfg:        hex(0xff628c),
		HoverBg:    hex(0x193549),
		Background: hex(0x0d1b26),
		Success:    hex(0x3ad900),
		Info:       hex(0x2affdf),
		Accent:     hex(0xff628c),
		Warning:    hex(0xffc600),
		CardHexes: [18]int32{
			0x2affdf, 0xffc600, 0x3ad900, 0xff9d00, 0xff628c, 0xff6d43, 0x80ffbb, 0x9effff,
			0xffc600, 0xff9d00, 0x2affdf, 0xff628c, 0x3ad900, 0x80ffbb, 0x9effff, 0xff6d43,
			0x8f9dbb, 0x2affdf,
		},
	},
	{
		Name:       "Poimandres",
		Text:       hex(0xe4f0ff),
		Muted:      hex(0x767c9d),
		Border:     hex(0x2b2e3b),
		Brand:      hex(0x5de4c7),
		Dev:        hex(0x89ddff),
		Mnt:        hex(0xfffac2),
		Cfg:        hex(0xd0679d),
		HoverBg:    hex(0x2b2e3b),
		Background: hex(0x1b1e28),
		Success:    hex(0x5de4c7),
		Info:       hex(0x89ddff),
		Accent:     hex(0xd0679d),
		Warning:    hex(0xfffac2),
		CardHexes: [18]int32{
			0x89ddff, 0x5de4c7, 0xadd7ff, 0xfffac2, 0xd0679d, 0xff9d97, 0x91b4d5, 0x5de4c7,
			0xd0679d, 0xfffac2, 0xadd7ff, 0x89ddff, 0x91b4d5, 0xff9d97, 0x5de4c7, 0xd0679d,
			0x767c9d, 0x89ddff,
		},
	},
	{
		Name:       "Ayu Mirage",
		Text:       hex(0xcbccc6),
		Muted:      hex(0x707a8a),
		Border:     hex(0x3d424d),
		Brand:      hex(0xffcc66),
		Dev:        hex(0x73d0ff),
		Mnt:        hex(0xffd580),
		Cfg:        hex(0xd4bfff),
		HoverBg:    hex(0x2d3440),
		Background: hex(0x1f2430),
		Success:    hex(0xbae67e),
		Info:       hex(0x95e6cb),
		Accent:     hex(0xd4bfff),
		Warning:    hex(0xffd580),
		CardHexes: [18]int32{
			0x73d0ff, 0x95e6cb, 0xbae67e, 0xffd580, 0xf28779, 0xffa759, 0xd4bfff, 0x5ccfe6,
			0xffcc66, 0xffd580, 0x73d0ff, 0xd4bfff, 0xbae67e, 0x95e6cb, 0xf28779, 0xffa759,
			0x707a8a, 0x73d0ff,
		},
	},
}

// Default es el tema por defecto (Compass).
func Default() *Theme { return Themes[0] }

// ByName busca un tema por nombre exacto (insensible a mayusculas y espacios).
func ByName(name string) (*Theme, bool) {
	needle := strings.TrimSpace(name)
	for _, t := range Themes {
		if strings.EqualFold(t.Name, needle) {
			return t, true
		}
	}
	return nil, false
}

// IndexOf devuelve el indice de un tema en Themes por nombre (0 si no existe).
func IndexOf(name string) int {
	needle := strings.TrimSpace(name)
	for i, t := range Themes {
		if strings.EqualFold(t.Name, needle) {
			return i
		}
	}
	return 0
}

// Next devuelve el tema siguiente en el ciclo (vuelve al primero tras el ultimo).
func Next(name string) *Theme {
	i := IndexOf(name)
	return Themes[(i+1)%len(Themes)]
}

// Prev devuelve el tema anterior en el ciclo.
func Prev(name string) *Theme {
	i := IndexOf(name)
	return Themes[(i+len(Themes)-1)%len(Themes)]
}

func configPath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return ""
	}
	return filepath.Join(base, "compass", "theme")
}

// Load devuelve el tema guardado por el usuario, o Default si no hay ninguno
// valido. Un fichero ilegible o con nombre desconocido no es un error.
func Load() *Theme {
	path := configPath()
	if path == "" {
		return Default()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Default()
	}
	if t, ok := ByName(string(data)); ok {
		return t
	}
	return Default()
}

// Save guarda el tema activo. Un fallo de escritura no es fatal.
func Save(t *Theme) {
	path := configPath()
	if path == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(t.Name), 0o644)
}
