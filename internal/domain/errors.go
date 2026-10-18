package domain

import "errors"

var (
	ErrNotFound          = errors.New("user not found")
	ErrSenderNotFound    = errors.New("sender not found")
	ErrRecipientNotFound = errors.New("recipient not found")
	ErrBalanceOverflow   = errors.New("balance overflow")
	ErrInsufficientFunds = errors.New("insufficient funds")

	ErrInvalidUserID = errors.New("user id must be positive")
	ErrInvalidAmount = errors.New("amount must be positive")
)
