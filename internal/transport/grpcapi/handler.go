package grpcapi

import (
	"context"
	"errors"

	balancev1 "github.com/irishout/balance-user-api/gen/go/balance/v1"
	"github.com/irishout/balance-user-api/internal/domain"
	"github.com/irishout/balance-user-api/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Handler struct {
	svc *service.Service
	balancev1.UnimplementedBalanceServiceServer
}

func NewHandler(s *service.Service) *Handler {
	return &Handler{
		svc: s,
	}
}

func toGRPCError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, domain.ErrBalanceOverflow):
		return status.Error(codes.OutOfRange, "balance overflow")
	case errors.Is(err, domain.ErrInvalidAmount):
		return status.Error(codes.InvalidArgument, "amount must be greater than 0")
	case errors.Is(err, domain.ErrInvalidUserID):
		return status.Error(codes.InvalidArgument, "user_id must be greater than 0")
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, "user_id not found")
	case errors.Is(err, domain.ErrInvalidSenderID):
		return status.Error(codes.InvalidArgument, "user_from_id must be greater than 0")
	case errors.Is(err, domain.ErrInvalidRecipientID):
		return status.Error(codes.InvalidArgument, "user_to_id must be greater than 0")
	case errors.Is(err, domain.ErrSelfTransfer):
		return status.Error(codes.InvalidArgument, "user_to_id and user_from_id must be different")
	case errors.Is(err, domain.ErrSenderNotFound):
		return status.Error(codes.NotFound, "user_from_id not found")
	case errors.Is(err, domain.ErrRecipientNotFound):
		return status.Error(codes.NotFound, "user_to_id not found")
	case errors.Is(err, domain.ErrInsufficientFunds):
		return status.Error(codes.FailedPrecondition, "insufficient funds")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "request canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "request deadline exceeded")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}

func (h *Handler) GetBalance(ctx context.Context, req *balancev1.BalanceRequest) (*balancev1.BalanceResponse, error) {
	userID := req.GetUserId()
	balance, err := h.svc.Balance(ctx, userID)
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &balancev1.BalanceResponse{
		Balance: balance,
	}, nil

}
