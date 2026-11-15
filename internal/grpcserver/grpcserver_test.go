package grpcserver

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/dismoralzor/metalert/internal/proto"
	"github.com/dismoralzor/metalert/internal/repository"
)

// startTestServer поднимает MetricsServer на случайном порту 127.0.0.1 с
// заданным interceptor-подсетью и возвращает клиента и хранилище.
func startTestServer(t *testing.T, subnet *net.IPNet) (pb.MetricsClient, *repository.MemStorage) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	storage := repository.NewMemStorage()
	srv := grpc.NewServer(grpc.UnaryInterceptor(TrustedSubnetInterceptor(subnet)))
	pb.RegisterMetricsServer(srv, NewMetricsServer(storage))
	go func() {
		_ = srv.Serve(lis)
	}()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return pb.NewMetricsClient(conn), storage
}

func testBatch() *pb.UpdateMetricsRequest {
	return &pb.UpdateMetricsRequest{Metrics: []*pb.Metric{
		{Id: "Alloc", Type: pb.Metric_GAUGE, Value: 42.5},
		{Id: "PollCount", Type: pb.Metric_COUNTER, Delta: 7},
	}}
}

func ctxWithRealIP(ip string) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "x-real-ip", ip)
}

func TestUpdateMetrics_SavesBatch(t *testing.T) {
	client, storage := startTestServer(t, nil)

	if _, err := client.UpdateMetrics(context.Background(), testBatch()); err != nil {
		t.Fatalf("UpdateMetrics: %v", err)
	}

	if v, ok := storage.GetGauge(context.Background(), "Alloc"); !ok || v != 42.5 {
		t.Fatalf("gauge Alloc = %v, %v; ожидалось 42.5, true", v, ok)
	}
	// Counter накапливается: вторая отправка того же батча удваивает delta.
	if _, err := client.UpdateMetrics(context.Background(), testBatch()); err != nil {
		t.Fatalf("UpdateMetrics second: %v", err)
	}
	if d, ok := storage.GetCounter(context.Background(), "PollCount"); !ok || d != 14 {
		t.Fatalf("counter PollCount = %v, %v; ожидалось 14, true", d, ok)
	}
}

func TestTrustedSubnetInterceptor(t *testing.T) {
	_, subnet, err := net.ParseCIDR("192.168.1.0/24")
	if err != nil {
		t.Fatalf("ParseCIDR: %v", err)
	}

	tests := []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"ip в подсети", ctxWithRealIP("192.168.1.10"), codes.OK},
		{"ip вне подсети", ctxWithRealIP("10.0.0.1"), codes.PermissionDenied},
		{"без метаданных", context.Background(), codes.PermissionDenied},
		{"кривой ip", ctxWithRealIP("not-an-ip"), codes.PermissionDenied},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := startTestServer(t, subnet)
			_, err := client.UpdateMetrics(tt.ctx, testBatch())
			if code := status.Code(err); code != tt.want {
				t.Fatalf("код = %v, ожидался %v (err = %v)", code, tt.want, err)
			}
		})
	}
}

func TestTrustedSubnetInterceptor_NilSubnet(t *testing.T) {
	client, _ := startTestServer(t, nil)

	// Подсеть не задана - пускаем даже без x-real-ip.
	if _, err := client.UpdateMetrics(context.Background(), testBatch()); err != nil {
		t.Fatalf("UpdateMetrics без подсети: %v", err)
	}
}

func TestFromProto(t *testing.T) {
	gauge := fromProto(&pb.Metric{Id: "G", Type: pb.Metric_GAUGE, Value: 1.5})
	if gauge.MType != "gauge" || gauge.Value == nil || *gauge.Value != 1.5 || gauge.Delta != nil {
		t.Fatalf("gauge конвертация: %+v", gauge)
	}

	counter := fromProto(&pb.Metric{Id: "C", Type: pb.Metric_COUNTER, Delta: 3})
	if counter.MType != "counter" || counter.Delta == nil || *counter.Delta != 3 || counter.Value != nil {
		t.Fatalf("counter конвертация: %+v", counter)
	}
}
