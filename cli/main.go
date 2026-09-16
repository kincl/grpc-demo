package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	doughnutv1 "github.com/jkincl/grpc-demo/gen/doughnut/v1"
	"github.com/jkincl/grpc-demo/gen/doughnut/v1/doughnutv1connect"
	"golang.org/x/net/http2"
)

func newHTTPClient(addr string) *http.Client {
	if strings.HasPrefix(addr, "https://") {
		// HTTPS: standard HTTP/2 with TLS
		return &http.Client{
			Transport: &http2.Transport{},
		}
	}
	// Plaintext: h2c for local dev
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		},
	}
}

type loggingInterceptor struct{}

func (loggingInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		method := req.Spec().Procedure
		fmt.Fprintf(os.Stderr, "→ gRPC %s\n", method)
		fmt.Fprintf(os.Stderr, "  REQ  %s\n", formatProto(req.Any()))
		resp, err := next(ctx, req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ERR  %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "  RESP %s\n", formatProto(resp.Any()))
		}
		return resp, err
	}
}

func (loggingInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (loggingInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

func formatProto(msg any) string {
	if m, ok := msg.(interface{ String() string }); ok {
		return m.String()
	}
	return fmt.Sprintf("%v", msg)
}

func main() {
	verbose := false
	args := os.Args[1:]
	for i, a := range args {
		if a == "-v" || a == "--verbose" {
			verbose = true
			args = append(args[:i], args[i+1:]...)
			break
		}
	}

	if len(args) < 1 {
		usage()
		os.Exit(1)
	}

	addr := os.Getenv("DOUGHNUT_ADDR")
	if addr == "" {
		addr = "http://localhost:50051"
	}

	opts := []connect.ClientOption{connect.WithGRPC()}
	if verbose {
		opts = append(opts, connect.WithInterceptors(loggingInterceptor{}))
	}

	client := doughnutv1connect.NewDoughnutServiceClient(
		newHTTPClient(addr),
		addr,
		opts...,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch args[0] {
	case "menu":
		doMenu(ctx, client)
	case "order":
		doOrder(ctx, client, args[1:])
	case "lucky":
		doLucky(ctx, client)
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: doughnut-cli <command>

Commands:
  menu                         List available flavors
  order <name> <flavor:qty>... Place an order (e.g. order Homer "Glazed:3" "Maple Bacon:2")
  lucky                        Place a random order

Flags:
  -v, --verbose  Log gRPC requests and responses to stderr

Environment:
  DOUGHNUT_ADDR  Backend address (default: http://localhost:50051)
`)
}

func doMenu(ctx context.Context, client doughnutv1connect.DoughnutServiceClient) {
	resp, err := client.ListFlavors(ctx, connect.NewRequest(&doughnutv1.ListFlavorsRequest{}))
	if err != nil {
		log.Fatalf("ListFlavors: %v", err)
	}
	fmt.Println("--- Red Hat Provisions Menu ---")
	for _, f := range resp.Msg.Flavors {
		fmt.Printf("  %-20s $%.2f  %s\n", f.Name, f.Price, f.Description)
	}
}

func doOrder(ctx context.Context, client doughnutv1connect.DoughnutServiceClient, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: doughnut-cli order <name> <flavor:qty>...")
		fmt.Fprintln(os.Stderr, "  Example: doughnut-cli order Homer \"Glazed:3\" \"Maple Bacon:2\"")
		os.Exit(1)
	}

	name := args[0]
	var items []*doughnutv1.OrderItem
	for _, arg := range args[1:] {
		parts := strings.SplitN(arg, ":", 2)
		if len(parts) != 2 {
			log.Fatalf("invalid item %q (expected flavor:quantity)", arg)
		}
		qty, err := strconv.Atoi(parts[1])
		if err != nil {
			log.Fatalf("invalid quantity %q for %s", parts[1], parts[0])
		}
		items = append(items, &doughnutv1.OrderItem{Flavor: parts[0], Quantity: int32(qty)})
	}

	resp, err := client.OrderDoughnuts(ctx, connect.NewRequest(&doughnutv1.OrderRequest{
		CustomerName: name,
		Items:        items,
	}))
	if err != nil {
		log.Fatalf("OrderDoughnuts: %v", err)
	}
	m := resp.Msg
	fmt.Printf("Order %s: %s\n", m.OrderId, m.Status)
	for _, item := range m.Items {
		fmt.Printf("  %dx %s\n", item.Quantity, item.Flavor)
	}
	fmt.Printf("Total: $%.2f\n", m.TotalPrice)
	fmt.Println(m.Message)
}

func doLucky(ctx context.Context, client doughnutv1connect.DoughnutServiceClient) {
	menuResp, err := client.ListFlavors(ctx, connect.NewRequest(&doughnutv1.ListFlavorsRequest{}))
	if err != nil {
		log.Fatalf("ListFlavors: %v", err)
	}

	names := []string{
		"Homer Simpson", "Bob Belcher", "Leslie Knope", "Ron Swanson",
		"Jake Peralta", "Dwight Schrute", "Ted Lasso", "Liz Lemon",
	}
	name := names[rand.Intn(len(names))]

	flavors := menuResp.Msg.Flavors
	count := 1 + rand.Intn(3)
	perm := rand.Perm(len(flavors))

	var items []*doughnutv1.OrderItem
	for i := 0; i < count && i < len(flavors); i++ {
		items = append(items, &doughnutv1.OrderItem{
			Flavor:   flavors[perm[i]].Name,
			Quantity: int32(1 + rand.Intn(6)),
		})
	}

	fmt.Printf("Ordering as %s...\n", name)
	for _, item := range items {
		fmt.Printf("  %dx %s\n", item.Quantity, item.Flavor)
	}

	resp, err := client.OrderDoughnuts(ctx, connect.NewRequest(&doughnutv1.OrderRequest{
		CustomerName: name,
		Items:        items,
	}))
	if err != nil {
		log.Fatalf("OrderDoughnuts: %v", err)
	}
	m := resp.Msg
	fmt.Printf("\nOrder %s: %s — Total: $%.2f\n", m.OrderId, m.Status, m.TotalPrice)
	fmt.Println(m.Message)
}
