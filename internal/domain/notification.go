package domain

import "time"

type WebhookPayload struct {
	ChamadoID      string    `json:"chamado_id"`
	Tipo           string    `json:"tipo"`
	CPF            string    `json:"cpf"`
	StatusAnterior string    `json:"status_anterior"`
	StatusNovo     string    `json:"status_novo"`
	Titulo         string    `json:"titulo"`
	Descricao      string    `json:"descricao"`
	Timestamp      time.Time `json:"timestamp"`
}

type Notification struct {
	ID             string    `json:"id"`
	ChamadoID      string    `json:"chamado_id"`
	Titulo         string    `json:"titulo"`
	Descricao      string    `json:"descricao"`
	StatusAnterior string    `json:"status_anterior"`
	StatusNovo     string    `json:"status_novo"`
	IsRead         bool      `json:"is_read"`
	CreatedAt      time.Time `json:"created_at"`
}
