package menu

// SubItem es una entrada de submenu: lo que la card necesita para pintarse.
type SubItem struct {
	Icon        string
	Label       string
	Description string
	// ColorKey es la clave de theme.CardKeys que da el acento de esta card.
	ColorKey string
}

// SubEntry asocia un valor del llamador con su card.
type SubEntry[T comparable] struct {
	Value T
	Item  SubItem
}
