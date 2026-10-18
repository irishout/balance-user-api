package domain

import "errors"

var (
	ErrNotFound          = errors.New("user not found")
	ErrSenderNotFound    = errors.New("sender not found")
	ErrRecipientNotFound = errors.New("recipient not found")
	ErrBalanceOverflow   = errors.New("balance overflow")
	ErrInsufficientFunds = errors.New("insufficient funds")

	ErrInvalidUserID      = errors.New("user id must be positive")
	ErrInvalidSenderID    = errors.New("sender id must be positive")
	ErrInvalidRecipientID = errors.New("recipient id must be positive")
	ErrInvalidAmount      = errors.New("amount must be positive")
	ErrSelfTransfer       = errors.New("user ids must be different")
)
