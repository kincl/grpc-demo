package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	doughnutv1 "github.com/jkincl/grpc-demo/gen/doughnut/v1"
	"github.com/jkincl/grpc-demo/gen/doughnut/v1/doughnutv1connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

var menu = []*doughnutv1.Flavor{
	{Name: "Glazed", Price: 1.50, Description: "Classic glazed doughnut"},
	{Name: "Maple Bacon", Price: 2.50, Description: "Maple frosted with crispy bacon bits"},
	{Name: "Boston Cream", Price: 2.00, Description: "Filled with custard, topped with chocolate"},
	{Name: "Jelly", Price: 1.75, Description: "Filled with strawberry jelly"},
	{Name: "Old Fashioned", Price: 1.50, Description: "Buttermilk cake doughnut"},
	{Name: "Chocolate Sprinkle", Price: 2.00, Description: "Chocolate frosted with rainbow sprinkles"},
}

type doughnutServer struct {
	priceMap map[string]float64
}

func newDoughnutServer() *doughnutServer {
	pm := make(map[string]float64, len(menu))
	for _, f := range menu {
		pm[strings.ToLower(f.Name)] = f.Price
	}
	return &doughnutServer{priceMap: pm}
}

func (s *doughnutServer) ListFlavors(_ context.Context, req *connect.Request[doughnutv1.ListFlavorsRequest]) (*connect.Response[doughnutv1.ListFlavorsResponse], error) {
	log.Printf("[ListFlavors] request received")
	resp := &doughnutv1.ListFlavorsResponse{Flavors: menu}
	log.Printf("[ListFlavors] returning %d flavors", len(resp.Flavors))
	return connect.NewResponse(resp), nil
}

func (s *doughnutServer) OrderDoughnuts(_ context.Context, req *connect.Request[doughnutv1.OrderRequest]) (*connect.Response[doughnutv1.OrderResponse], error) {
	r := req.Msg
	log.Printf("[OrderDoughnuts] request: customer=%q items=%v", r.CustomerName, r.Items)

	if r.CustomerName == "" {
		log.Printf("[OrderDoughnuts] error: empty customer name")
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("customer name is required"))
	}
	if len(r.Items) == 0 {
		log.Printf("[OrderDoughnuts] error: no items")
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("at least one item is required"))
	}

	var total float64
	for _, item := range r.Items {
		price, ok := s.priceMap[strings.ToLower(item.Flavor)]
		if !ok {
			log.Printf("[OrderDoughnuts] error: unknown flavor %q", item.Flavor)
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unknown flavor: %s", item.Flavor))
		}
		if item.Quantity <= 0 {
			log.Printf("[OrderDoughnuts] error: invalid quantity %d for %q", item.Quantity, item.Flavor)
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("quantity must be positive for %s", item.Flavor))
		}
		total += price * float64(item.Quantity)
	}

	orderID := fmt.Sprintf("RHP-%05d", rand.Intn(100000))
	resp := &doughnutv1.OrderResponse{
		OrderId:    orderID,
		Status:     "CONFIRMED",
		TotalPrice: total,
		Message:    fmt.Sprintf("Thank you, %s! Your doughnuts will be ready shortly.", r.CustomerName),
		Items:      r.Items,
	}

	log.Printf("[OrderDoughnuts] response: order_id=%s total=$%.2f items=%d", resp.OrderId, resp.TotalPrice, len(resp.Items))
	return connect.NewResponse(resp), nil
}

func corsHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Grpc-Web, X-User-Agent")
		w.Header().Set("Access-Control-Expose-Headers", "Grpc-Status, Grpc-Message")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "50051"
	}

	srv := newDoughnutServer()
	mux := http.NewServeMux()

	path, handler := doughnutv1connect.NewDoughnutServiceHandler(srv)
	mux.Handle(path, handler)

	reflector := grpcreflect.NewStaticReflector(doughnutv1connect.DoughnutServiceName)
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           h2c.NewHandler(corsHandler(mux), &http2.Server{}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	log.Printf("Red Hat Provisions backend listening on :%s (gRPC + gRPC-Web + Connect)", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
