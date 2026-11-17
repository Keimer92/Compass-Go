package gui

import (
	"compass/internal/tui"

	"gioui.org/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

// cardIconData asocia cada card con un icono vectorial real (Material Design)
// en vez de la inicial del titulo.
var cardIconData = map[tui.MenuItem][]byte{
	tui.WindowsDev:       icons.AVPlayArrow,
	tui.AndroidDev:       icons.ActionAndroid,
	tui.AndroidHmr:       icons.ActionAutorenew,
	tui.WindowsBuild:     icons.ActionBuild,
	tui.AndroidBuildArm64: icons.ActionAssignment,
	tui.AndroidBuildArmv7: icons.ActionAssignment,
	tui.AndroidBuildAll:   icons.MapsLayers,
	tui.InstallDebug:      icons.ActionGetApp,
	tui.FlutterProfile:    icons.ToggleStar,
	tui.Tests:             icons.ActionBugReport,
	tui.Clean:             icons.ActionDelete,
	tui.DebugAndroid:      icons.ActionSearch,
	tui.Signing:           icons.CommunicationVPNKey,
	tui.VersionItem:       icons.ActionUpdate,
	tui.Git:               icons.ActionCode,
	tui.BuildWhatsApp:     icons.CommunicationChat,
	tui.ConfigureTerminal: icons.ActionDescription,
	tui.Environment:       icons.ActionSettings,
}

// iconsByItem cachea los widget.Icon (cada uno rasteriza su imagen).
var iconsByItem = buildIcons()

func buildIcons() map[tui.MenuItem]*widget.Icon {
	out := make(map[tui.MenuItem]*widget.Icon, len(cardIconData))
	for item, data := range cardIconData {
		if ic, err := widget.NewIcon(data); err == nil {
			out[item] = ic
		}
	}
	return out
}
