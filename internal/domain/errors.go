package domain

import "errors"

var (
	ErrNotFound          = errors.New("user not found")
	ErrBalanceOverflow   = errors.New("balance overflow")
	ErrInsufficientFunds = errors.New("insufficient funds")
)
