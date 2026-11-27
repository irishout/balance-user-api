package grpcapi

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	balancev1 "github.com/irishout/balance-user-api/gen/go/balance/v1"
	"github.com/irishout/balance-user-api/internal/domain"
	"github.com/irishout/balance-user-api/internal/service"
	"github.com/irishout/balance-user-api/internal/storage/local"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

type operationsQuery struct {
	userID int64
	limit  int
	offset int
	sortBy string
}

type operationsStorageStub struct {
	service.Storage
	queries chan operationsQuery
	history []domain.Operation
}

func (s *operationsStorageStub) ListOperations(ctx context.Context, userID int64, limit, offset int, sortBy string) ([]domain.Operation, error) {
	select {
	case s.queries <- operationsQuery{userID: userID, limit: limit, offset: offset, sortBy: sortBy}:
		return s.history, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func newOperationsClient(t *testing.T, storage service.Storage) balancev1.BalanceServiceClient {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	balancev1.RegisterBalanceServiceServer(server, NewHandler(service.NewService(storage)))
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		if err := <-serveErr; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("serve gRPC: %v", err)
		}
	})

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
	)
	if err != nil {
		t.Fatalf("create gRPC client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return balancev1.NewBalanceServiceClient(conn)
}

func TestOperationsPagination(t *testing.T) {
	date := time.Date(2026, time.November, 26, 18, 0, 0, 123000000, time.FixedZone("UTC+3", 3*60*60))
	history := []domain.Operation{
		{ID: 3, Type: "transfer", Date: date, UserID: 1, RecipientID: proto.Int64(2), Amount: 300, Comment: "Возврат долга"},
		{ID: 2, Type: "deposit", Date: date, UserID: 2, Amount: 100},
	}
	wantOperations := &balancev1.OperationsResponse{Operations: []*balancev1.OperationsResponse_Operation{
		{Id: 3, OperationType: "transfer", OperationDate: "2026-11-26T15:00:00.123Z", UserId: 1, RecipientId: proto.Int64(2), Amount: 300, Comment: "Возврат долга"},
		{Id: 2, OperationType: "deposit", OperationDate: "2026-11-26T15:00:00.123Z", UserId: 2, Amount: 100},
	}}
	tests := []struct {
		name      string
		request   *balancev1.OperationsRequest
		wantQuery operationsQuery
		history   []domain.Operation
		want      *balancev1.OperationsResponse
	}{
		{
			name: "defaults", request: &balancev1.OperationsRequest{UserId: 2},
			wantQuery: operationsQuery{userID: 2, limit: 20, sortBy: "date"}, history: history, want: wantOperations,
		},
		{
			name: "only page size", request: &balancev1.OperationsRequest{UserId: 2, PageSize: proto.Int32(2)},
			wantQuery: operationsQuery{userID: 2, limit: 2, sortBy: "date"}, history: history, want: wantOperations,
		},
		{
			name: "only page", request: &balancev1.OperationsRequest{UserId: 2, Page: proto.Int32(2)},
			wantQuery: operationsQuery{userID: 2, limit: 20, offset: 20, sortBy: "date"}, want: &balancev1.OperationsResponse{},
		},
		{
			name: "custom page with amount sort", request: &balancev1.OperationsRequest{UserId: 2, Page: proto.Int32(3), PageSize: proto.Int32(2), Sort: "amount"},
			wantQuery: operationsQuery{userID: 2, limit: 2, offset: 4, sortBy: "amount"}, want: &balancev1.OperationsResponse{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			storage := &operationsStorageStub{queries: make(chan operationsQuery, 1), history: test.history}
			client := newOperationsClient(t, storage)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()

			response, err := client.Operations(ctx, test.request)
			if err != nil {
				t.Fatalf("Operations: %v", err)
			}
			if !proto.Equal(response, test.want) {
				t.Errorf("response = %v, want %v", response, test.want)
			}
			select {
			case query := <-storage.queries:
				if query != test.wantQuery {
					t.Errorf("storage query = %+v, want %+v", query, test.wantQuery)
				}
			default:
				t.Error("storage did not receive a query")
			}
		})
	}
}

func TestOperationsErrors(t *testing.T) {
	client := newOperationsClient(t, local.NewMapStorage())
	tests := []struct {
		name    string
		request *balancev1.OperationsRequest
		code    codes.Code
		message string
	}{
		{"invalid user", &balancev1.OperationsRequest{}, codes.InvalidArgument, "user_id must be greater than 0"},
		{"zero page", &balancev1.OperationsRequest{UserId: 2, Page: proto.Int32(0)}, codes.InvalidArgument, "page must be positive"},
		{"negative page", &balancev1.OperationsRequest{UserId: 2, Page: proto.Int32(-1)}, codes.InvalidArgument, "page must be positive"},
		{"zero page size", &balancev1.OperationsRequest{UserId: 2, PageSize: proto.Int32(0)}, codes.InvalidArgument, "page_size must be positive"},
		{"negative page size", &balancev1.OperationsRequest{UserId: 2, PageSize: proto.Int32(-1)}, codes.InvalidArgument, "page_size must be positive"},
		{"invalid sort", &balancev1.OperationsRequest{UserId: 2, Sort: "foo"}, codes.InvalidArgument, "sort must be date or amount"},
		{"unsupported storage", &balancev1.OperationsRequest{UserId: 2}, codes.Unimplemented, "operation history requires PostgreSQL"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()

			response, err := client.Operations(ctx, test.request)
			if response != nil {
				t.Errorf("expected nil response, got %v", response)
			}
			requireGRPCError(t, err, test.code, test.message)
		})
	}
}

type operationComment struct {
	operation string
	comment   string
}

type commentStorageStub struct {
	service.Storage
	received chan operationComment
}

func (s *commentStorageStub) Deposit(_ context.Context, _, _ int64, comment string) (int64, error) {
	s.received <- operationComment{operation: "deposit", comment: comment}
	return 200, nil
}

func (s *commentStorageStub) Withdraw(_ context.Context, _, _ int64, comment string) (int64, error) {
	s.received <- operationComment{operation: "withdraw", comment: comment}
	return 100, nil
}

func (s *commentStorageStub) Transfer(_ context.Context, _, _, _ int64, comment string) (int64, int64, error) {
	s.received <- operationComment{operation: "transfer", comment: comment}
	return 100, 200, nil
}

func TestGRPCOperationComments(t *testing.T) {
	tests := []struct {
		name    string
		comment string
	}{
		{name: "with comment", comment: "Оплата O'Reilly, заказ №1"},
		{name: "without comment"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			storage := &commentStorageStub{received: make(chan operationComment, 3)}
			client := newOperationsClient(t, storage)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()

			_, err := client.Deposit(ctx, &balancev1.DepositRequest{UserId: 1, Amount: 100, Comment: test.comment})
			requireOperationComment(t, storage.received, "deposit", test.comment, err)

			_, err = client.Withdraw(ctx, &balancev1.WithdrawRequest{UserId: 1, Amount: 100, Comment: test.comment})
			requireOperationComment(t, storage.received, "withdraw", test.comment, err)

			_, err = client.Transfer(ctx, &balancev1.TransferRequest{UserFromId: 1, UserToId: 2, Amount: 100, Comment: test.comment})
			requireOperationComment(t, storage.received, "transfer", test.comment, err)
		})
	}
}

func requireOperationComment(t *testing.T, received <-chan operationComment, operation, comment string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", operation, err)
	}
	select {
	case got := <-received:
		if got.operation != operation || got.comment != comment {
			t.Fatalf("storage got %+v, want operation=%q, comment=%q", got, operation, comment)
		}
	default:
		t.Fatalf("%s did not call storage", operation)
	}
}
