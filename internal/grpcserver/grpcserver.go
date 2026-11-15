// Package grpcserver - gRPC-фасад к хранилищу метрик: сервис Metrics из
// metrics.proto и interceptor проверки доверенной подсети. HTTP- и gRPC-приём
// метрик делят одно хранилище - бизнес-логика одна, отличается только транспорт.
package grpcserver

import (
	"context"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	models "github.com/dismoralzor/metalert/internal/model"
	pb "github.com/dismoralzor/metalert/internal/proto"
	"github.com/dismoralzor/metalert/internal/repository"
)

// MetricsServer реализует pb.MetricsServer поверх того же Storage, что
// обслуживает HTTP-хендлеры.
type MetricsServer struct {
	pb.UnimplementedMetricsServer
	storage repository.Storage
}

// NewMetricsServer собирает сервер на заданном хранилище.
func NewMetricsServer(storage repository.Storage) *MetricsServer {
	return &MetricsServer{storage: storage}
}

// UpdateMetrics принимает батч метрик и сохраняет его. Ошибка хранилища
// уходит клиенту как codes.Internal - разбираться с ней стоит на сервере,
// агенту достаточно знать, что батч не принят.
func (s *MetricsServer) UpdateMetrics(ctx context.Context, req *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error) {
	metrics := make([]models.Metrics, 0, len(req.GetMetrics()))
	for _, m := range req.GetMetrics() {
		metrics = append(metrics, fromProto(m))
	}
	if err := s.storage.UpdateBatch(ctx, metrics); err != nil {
		return nil, status.Errorf(codes.Internal, "save metrics: %v", err)
	}
	return &pb.UpdateMetricsResponse{}, nil
}

// fromProto конвертирует proto-метрику во внутреннюю модель. Тип определяет,
// какое поле заполнено: для gauge - Value, для counter - Delta (как в
// JSON-протоколе, там эти поля тоже указатели).
func fromProto(m *pb.Metric) models.Metrics {
	out := models.Metrics{ID: m.GetId()}
	switch m.GetType() {
	case pb.Metric_GAUGE:
		out.MType = models.Gauge
		v := m.GetValue()
		out.Value = &v
	case pb.Metric_COUNTER:
		out.MType = models.Counter
		d := m.GetDelta()
		out.Delta = &d
	}
	return out
}

// TrustedSubnetInterceptor - унарный interceptor проверки доверенной подсети:
// агент передаёт свой IP в метаданных x-real-ip. subnet == nil (опция
// trusted_subnet не задана) - ограничений нет. Аналог TrustedSubnetMiddleware
// из HTTP-слоя, только ошибка не 403, а codes.PermissionDenied.
func TrustedSubnetInterceptor(subnet *net.IPNet) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if subnet != nil {
			var ipStr string
			if md, ok := metadata.FromIncomingContext(ctx); ok {
				if vals := md.Get("x-real-ip"); len(vals) > 0 {
					ipStr = vals[0]
				}
			}
			ip := net.ParseIP(ipStr)
			if ip == nil || !subnet.Contains(ip) {
				return nil, status.Errorf(codes.PermissionDenied, "ip %q is not in trusted subnet", ipStr)
			}
		}
		return handler(ctx, req)
	}
}
