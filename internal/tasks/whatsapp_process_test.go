package tasks

import "testing"

func TestIsProjectWhatsappEnginePath(t *testing.T) {
	root := `D:\Mis Proyectos\Flutter\ELA Group`
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"staging", `D:\Mis Proyectos\Flutter\ELA Group\whatsapp_engine\node.exe`, true},
		{"release x64", `D:\Mis Proyectos\Flutter\ELA Group\build\windows\x64\runner\Release\whatsapp_engine\node.exe`, true},
		{"bundle go", `D:\Mis Proyectos\Flutter\ELA Group\resources\whatsapp-engine-bundle\node.exe`, true},
		{"slash mezclados", `D:/Mis Proyectos/Flutter/ELA Group/whatsapp_engine/node.exe`, true},
		{"node del sistema", `C:\Program Files\nodejs\node.exe`, false},
		{"otro proyecto", `D:\Mis Proyectos\Flutter\Otro\whatsapp_engine\node.exe`, false},
		{"prefijo engañoso", `D:\Mis Proyectos\Flutter\ELA Group 2\whatsapp_engine\node.exe`, false},
		{"exe de la app", `D:\Mis Proyectos\Flutter\ELA Group\build\windows\x64\runner\Release\ela.exe`, false},
		{"porteria vacia", ``, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isProjectWhatsappEnginePath(tc.path, root); got != tc.want {
				t.Fatalf("isProjectWhatsappEnginePath(%q) = %v, quiere %v", tc.path, got, tc.want)
			}
		})
	}
}
