package domain

import "errors"

var (
	ErrNotFound = errors.New("user not found")

	ErrSenderNotFound = errors.New("user_from_id not found")

	ErrRecipientNotFound = errors.New("user_to_id not found")

	ErrBalanceOverflow = errors.New("balance overflow")

	ErrInsufficientFunds = errors.New("insufficient funds")
)
