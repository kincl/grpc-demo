IMAGE_REGISTRY ?= ghcr.io/jkincl/grpc-demo
IMAGE_TAG ?= latest

BACKEND_IMAGE = $(IMAGE_REGISTRY)/doughnut-backend:$(IMAGE_TAG)
FRONTEND_IMAGE = $(IMAGE_REGISTRY)/doughnut-frontend:$(IMAGE_TAG)

.PHONY: proto build push deploy undeploy clean run-local

proto:
	protoc --proto_path=proto \
		--go_out=gen --go_opt=paths=source_relative \
		--connect-go_out=gen --connect-go_opt=paths=source_relative \
		doughnut/v1/doughnut.proto
	cp proto/doughnut/v1/doughnut.proto frontend/static/doughnut.proto

build:
	docker build -t $(BACKEND_IMAGE) -f backend/Dockerfile .
	docker build -t $(FRONTEND_IMAGE) frontend/

push: build
	docker push $(BACKEND_IMAGE)
	docker push $(FRONTEND_IMAGE)

deploy:
	kubectl apply -f k8s/

undeploy:
	kubectl delete -f k8s/ --ignore-not-found

run-local:
	@echo "Starting backend on :50051 (gRPC + gRPC-Web)..."
	@go run ./backend &
	@sleep 1
	@echo "Serving frontend on :8080..."
	@echo "Open http://localhost:8080?backend=http://localhost:50051"
	@cd frontend/static && python3 -m http.server 8080

clean:
	rm -f backend/doughnut-backend
