package grpcapi

import (
	"context"
	"errors"
	"time"

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
	case errors.Is(err, domain.ErrInvalidPagination):
		return status.Error(codes.InvalidArgument, "invalid pagination")
	case errors.Is(err, domain.ErrInvalidOperationSort):
		return status.Error(codes.InvalidArgument, "sort must be date or amount")
	case errors.Is(err, errors.ErrUnsupported):
		return status.Error(codes.Unimplemented, "operation history requires PostgreSQL")
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

func (h *Handler) Deposit(ctx context.Context, req *balancev1.DepositRequest) (*balancev1.BalanceResponse, error) {
	balance, err := h.svc.Deposit(ctx, req.GetUserId(), req.GetAmount())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &balancev1.BalanceResponse{
		Balance: balance,
	}, nil

}

func (h *Handler) Withdraw(ctx context.Context, req *balancev1.WithdrawRequest) (*balancev1.BalanceResponse, error) {
	balance, err := h.svc.Withdraw(ctx, req.GetUserId(), req.GetAmount())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &balancev1.BalanceResponse{
		Balance: balance,
	}, nil

}

func (h *Handler) Transfer(ctx context.Context, req *balancev1.TransferRequest) (*balancev1.TransferResponse, error) {
	balanceFrom, balanceTo, err := h.svc.Transfer(ctx, req.GetUserFromId(), req.GetUserToId(), req.GetAmount())
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &balancev1.TransferResponse{
		BalanceUserFrom: balanceFrom,
		BalanceUserTo:   balanceTo,
	}, nil

}

func (h *Handler) Operations(ctx context.Context, req *balancev1.OperationsRequest) (*balancev1.OperationsResponse, error) {
	if req == nil {
		return nil, toGRPCError(domain.ErrInvalidUserID)
	}

	page, pageSize := 1, 20
	if req.Page != nil {
		page = int(req.GetPage())
	}
	if req.PageSize != nil {
		pageSize = int(req.GetPageSize())
	}
	if page < 1 {
		return nil, status.Error(codes.InvalidArgument, "page must be positive")
	}
	if pageSize < 1 {
		return nil, status.Error(codes.InvalidArgument, "page_size must be positive")
	}
	offset := (page - 1) * pageSize

	history, err := h.svc.ListOperations(ctx, req.GetUserId(), pageSize, offset, req.GetSort())
	if err != nil {
		return nil, toGRPCError(err)
	}

	operations := make([]*balancev1.OperationsResponse_Operation, 0, len(history))
	for _, op := range history {
		operations = append(operations, &balancev1.OperationsResponse_Operation{
			Id:            op.ID,
			OperationType: op.Type,
			OperationDate: op.Date.UTC().Format(time.RFC3339Nano),
			UserId:        op.UserID,
			RecipientId:   op.RecipientID,
			Amount:        op.Amount,
			Comment:       op.Comment,
		})
	}

	return &balancev1.OperationsResponse{
		Operations: operations,
	}, nil
}
