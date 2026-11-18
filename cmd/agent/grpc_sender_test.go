package main

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/dismoralzor/metalert/internal/grpcserver"
	models "github.com/dismoralzor/metalert/internal/model"
	pb "github.com/dismoralzor/metalert/internal/proto"
	"github.com/dismoralzor/metalert/internal/repository"
	"github.com/dismoralzor/metalert/internal/retry"
)

func TestToProto(t *testing.T) {
	v := 1.5
	gauge := toProto(models.Metrics{ID: "G", MType: models.Gauge, Value: &v})
	if gauge.GetType() != pb.Metric_GAUGE || gauge.GetValue() != 1.5 {
		t.Fatalf("gauge конвертация: %+v", gauge)
	}

	d := int64(3)
	counter := toProto(models.Metrics{ID: "C", MType: models.Counter, Delta: &d})
	if counter.GetType() != pb.Metric_COUNTER || counter.GetDelta() != 3 {
		t.Fatalf("counter конвертация: %+v", counter)
	}
}

func TestIsRetriableGRPC(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"недоступен - повтор", status.Error(codes.Unavailable, "down"), true},
		{"нет ресурсов - повтор", status.Error(codes.ResourceExhausted, "slow"), true},
		{"дедлайн - повтор", status.Error(codes.DeadlineExceeded, "slow"), true},
		{"запрет - не повторяем", status.Error(codes.PermissionDenied, "denied"), false},
		{"внутренняя - не повторяем", status.Error(codes.Internal, "boom"), false},
		{"не-gRPC ошибка - не повторяем", errors.New("net down"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetriableGRPC(tt.err); got != tt.want {
				t.Fatalf("isRetriableGRPC = %v, ожидалось %v", got, tt.want)
			}
		})
	}
}

// TestGRPCSender_Send проверяет сквозной путь: grpcSender -> interceptor ->
// MetricsServer -> хранилище.
func TestGRPCSender_Send(t *testing.T) {
	_, subnet, err := net.ParseCIDR("127.0.0.0/8")
	if err != nil {
		t.Fatalf("ParseCIDR: %v", err)
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	storage := repository.NewMemStorage()
	srv := grpc.NewServer(grpc.UnaryInterceptor(grpcserver.TrustedSubnetInterceptor(subnet)))
	pb.RegisterMetricsServer(srv, grpcserver.NewMetricsServer(storage))
	go func() {
		_ = srv.Serve(lis)
	}()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	s := &grpcSender{
		conn:   conn,
		client: pb.NewMetricsClient(conn),
		policy: retry.Policy{},
		realIP: "127.0.0.1",
	}

	v := 1.5
	d := int64(2)
	batch := []models.Metrics{
		{ID: "G", MType: models.Gauge, Value: &v},
		{ID: "C", MType: models.Counter, Delta: &d},
	}

	if !s.Send(context.Background(), batch) {
		t.Fatal("Send вернул false")
	}
	if got, ok := storage.GetGauge(context.Background(), "G"); !ok || got != 1.5 {
		t.Fatalf("gauge G = %v, %v; ожидалось 1.5, true", got, ok)
	}
	if got, ok := storage.GetCounter(context.Background(), "C"); !ok || got != 2 {
		t.Fatalf("counter C = %v, %v; ожидалось 2, true", got, ok)
	}

	// IP вне доверенной подсети - сервер отказывает, Send возвращает false.
	s.realIP = "10.0.0.1"
	if s.Send(context.Background(), batch) {
		t.Fatal("Send с чужим IP вернул true, ожидался PermissionDenied")
	}
}

// hangingMetricsServer - сервис, чей UpdateMetrics висит, пока клиент не
// отменит вызов: нужен для проверки дедлайна на стороне сендера.
type hangingMetricsServer struct {
	pb.UnimplementedMetricsServer
}

func (h *hangingMetricsServer) UpdateMetrics(ctx context.Context, _ *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestGRPCSender_Deadline: без дедлайна зависший RPC блокировал бы воркер
// навсегда - с дедлайном SendOnce возвращает DeadlineExceeded за rpcTimeout.
func TestGRPCSender_Deadline(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	pb.RegisterMetricsServer(srv, &hangingMetricsServer{})
	go func() {
		_ = srv.Serve(lis)
	}()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	s := &grpcSender{conn: conn, client: pb.NewMetricsClient(conn), realIP: "127.0.0.1"}

	v := 1.5
	batch := []models.Metrics{{ID: "G", MType: models.Gauge, Value: &v}}

	start := time.Now()
	err = s.SendOnce(context.Background(), batch)
	elapsed := time.Since(start)

	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("err = %v, ожидался DeadlineExceeded", err)
	}
	// Грубые рамки: не раньше дедлайна и не "навсегда" (с запасом на CI).
	if elapsed < rpcTimeout || elapsed > rpcTimeout+3*time.Second {
		t.Fatalf("дедлайн сработал за %v, ожидалось около %v", elapsed, rpcTimeout)
	}
}
