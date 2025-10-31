dev:
	# Prerequisites: go install github.com/air-verse/air@latest && brew install overmind redis
	# Starts: Redis + API server + Worker + Asynqmon (all with hot reload)
	# Asynqmon available at http://localhost:8090/asynqmon
	overmind start

# Build CLI binary
build-server:
	go build -o bin/server ./cmd/server

build-cli:
	go build -o bin/branchd ./cmd/cli

# Generate OpenAPI spec and TypeScript types
openapi:
	@echo "Generating OpenAPI specification..."
	swag init -g internal/server/server.go --parseDependency --parseInternal --ot json,yaml --output _swagger
	@echo "Generating TypeScript types..."
	cd web && bunx swagger-typescript-api generate -p ../_swagger/swagger.json -o src/lib -n openapi.ts
	@echo "Removing comments from generated types..."
	sed -i '' '/\/\*\*.*\*\//d' web/src/lib/openapi.ts
	sed -i '' '/\/\*\*/,/\*\//d' web/src/lib/openapi.ts
	@echo "OpenAPI generation complete!"

# Upload CloudFormation template to S3
upload-cloudformation:
	./bin/upload_cloudformation.sh
