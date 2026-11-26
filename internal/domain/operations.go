package domain

import "time"

type Operation struct {
	ID          int64
	Type        string
	Date        time.Time
	UserID      int64
	RecipientID *int64
	Amount      int64
	Comment     string
}
