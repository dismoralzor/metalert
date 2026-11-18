package main

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	models "github.com/dismoralzor/metalert/internal/model"
	pb "github.com/dismoralzor/metalert/internal/proto"
	"github.com/dismoralzor/metalert/internal/retry"
)

// rpcTimeout - дедлайн одной попытки UpdateMetrics. Без него RPC при зависшем
// соединении мог бы заблокировать воркер на неопределённое время, и повторы
// по политике не сработали бы. По величине соответствует таймауту HTTP-клиента.
const rpcTimeout = 5 * time.Second

// grpcSender - отправитель батчей по gRPC, альтернатива HTTP-sender'у.
// Реализует тот же batchSender, поэтому аккумулятор и воркеры не замечают
// разницы. Повторы - та же retry.Policy, но с классификацией по gRPC-кодам.
type grpcSender struct {
	conn   *grpc.ClientConn
	client pb.MetricsClient
	policy retry.Policy
	// realIP - IP хоста агента для метаданных x-real-ip (сервер может
	// ограничивать приём доверенной подсетью - см. interceptor).
	realIP string
}

// newGRPCSender подключается к gRPC-серверу метрик по адресу addr (host:port,
// без схемы). Соединение ленивое: grpc.NewClient не дозванивается до первого
// запроса, поэтому неработающий адрес обнаружится на первой отправке.
func newGRPCSender(addr string, policy retry.Policy) *grpcSender {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		// NewClient падает только на синтаксически кривом адресе - это ошибка
		// конфигурации, смысла продолжать нет.
		log.Fatalf("grpc client: %v", err)
	}
	return &grpcSender{
		conn:   conn,
		client: pb.NewMetricsClient(conn),
		policy: policy,
		realIP: localIP(addr),
	}
}

// Close закрывает соединение при остановке агента.
func (s *grpcSender) Close() {
	_ = s.conn.Close()
}

// Send отправляет батч с обычной политикой повторов (см. sender.Send).
func (s *grpcSender) Send(ctx context.Context, metrics []models.Metrics) bool {
	if err := s.send(ctx, s.policy, metrics); err != nil {
		log.Printf("send batch via grpc: %v", err)
		return false
	}
	return true
}

// SendOnce делает ровно одну попытку (финальная отправка при shutdown).
func (s *grpcSender) SendOnce(ctx context.Context, metrics []models.Metrics) error {
	return s.send(ctx, retry.Policy{}, metrics)
}

// send конвертирует батч в UpdateMetricsRequest и вызывает UpdateMetrics,
// подкладывая IP агента в метаданные x-real-ip. Каждая попытка ограничена
// дедлайном rpcTimeout (у финальной отправки при shutdown свой, более
// короткий - он и сработает).
func (s *grpcSender) send(ctx context.Context, policy retry.Policy, metrics []models.Metrics) error {
	req := &pb.UpdateMetricsRequest{Metrics: make([]*pb.Metric, 0, len(metrics))}
	for _, m := range metrics {
		req.Metrics = append(req.Metrics, toProto(m))
	}

	ctx = metadata.AppendToOutgoingContext(ctx, "x-real-ip", s.realIP)
	return policy.Do(ctx, func() error {
		callCtx, cancel := context.WithTimeout(ctx, rpcTimeout)
		defer cancel()
		_, err := s.client.UpdateMetrics(callCtx, req)
		return err
	}, isRetriableGRPC)
}

// isRetriableGRPC классифицирует ошибки gRPC: временные (недоступность,
// превышение ресурсов, тайм-аут) - повторяем; PermissionDenied и прочие
// логические - бессмысленно.
func isRetriableGRPC(err error) bool {
	switch status.Code(err) {
	case codes.Unavailable, codes.ResourceExhausted, codes.DeadlineExceeded, codes.Aborted:
		return true
	}
	return false
}

// toProto конвертирует внутреннюю метрику в proto-вид (зеркально fromProto
// на сервере).
func toProto(m models.Metrics) *pb.Metric {
	out := &pb.Metric{Id: m.ID}
	switch m.MType {
	case models.Gauge:
		out.Type = pb.Metric_GAUGE
		if m.Value != nil {
			out.Value = *m.Value
		}
	case models.Counter:
		out.Type = pb.Metric_COUNTER
		if m.Delta != nil {
			out.Delta = *m.Delta
		}
	}
	return out
}
