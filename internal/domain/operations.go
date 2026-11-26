package domain

import "time"

type Operation struct {
	ID          int64     `json:"id"`
	Type        string    `json:"operation_type"`
	Date        time.Time `json:"operation_date"`
	UserID      int64     `json:"user_id"`
	RecipientID *int64    `json:"recipient_id"`
	Amount      int64     `json:"amount"`
	Comment     string    `json:"comment"`
}
