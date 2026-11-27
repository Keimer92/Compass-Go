package whatsapp

// Status es el estado del motor tal como lo consume la UI. Los nombres de
// campo JSON son el contrato con los frontends existentes (GymPro los usa
// directamente en su modelo), así que no se renombran.
type Status struct {
	Status     string  `json:"status"`
	QR         *string `json:"qr"`
	Message    *string `json:"message"`
	ClientInfo *string `json:"client_info"`
}
